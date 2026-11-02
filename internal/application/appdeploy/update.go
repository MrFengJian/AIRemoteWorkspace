package appdeploy

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ai-remote/workspace/internal/domain"
)

// 目录更新的网络预算。
const (
	httpTimeout    = 30 * time.Second
	maxCatalogSize = 16 << 10 // catalog.json 上限
	maxAssetSize   = 32 << 20 // catalog.zip 上限
	maxFileSize    = 2 << 20  // 单文件（app.json/guide.md/icon.svg）上限
	githubAPIBase  = "https://api.github.com"
	githubRawBase  = "https://raw.githubusercontent.com"
)

// repoName 返回更新源仓库：用户设置优先，空则默认仓库。
func (s *Service) repoName() string {
	if s.repoFn != nil {
		if r := strings.TrimSpace(s.repoFn()); r != "" {
			return r
		}
	}
	return domain.DefaultCatalogRepo
}

// ghGet 带 GitHub Accept 头的 GET（默认 http.Client，30s 超时）。
func ghGet(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "ai-remote-workspace")
	return http.DefaultClient.Do(req)
}

// CheckCatalogUpdate 检查目录更新：release = 最新 release 的 catalog.zip，
// main = main 分支 apps/catalog/catalog.json。两种来源都返回版本对比。
func (s *Service) CheckCatalogUpdate(ctx context.Context, source string) (domain.CatalogUpdateCheck, error) {
	repo := s.repoName()
	current, _, err := s.CatalogVersion()
	if err != nil {
		current = 0
	}
	check := domain.CatalogUpdateCheck{Source: source, Repo: repo, CurrentVersion: current}
	switch source {
	case domain.AppCatalogSourceRelease:
		rel, err := fetchLatestRelease(ctx, repo)
		if err != nil {
			return check, err
		}
		assetURL := findCatalogAsset(rel)
		if assetURL == "" {
			return check, fmt.Errorf("release %q 未找到 catalog.zip 资产", rel.TagName)
		}
		files, err := downloadZipFiles(ctx, assetURL)
		if err != nil {
			return check, err
		}
		version, err := catalogVersionOf(files)
		if err != nil {
			return check, err
		}
		check.LatestVersion = version
		check.Ref = rel.TagName
		check.Notes = rel.Notes
	case domain.AppCatalogSourceMain:
		files, err := fetchBranchCatalog(ctx, repo, "main")
		if err != nil {
			return check, err
		}
		version, err := catalogVersionOf(files)
		if err != nil {
			return check, err
		}
		check.LatestVersion = version
		check.Ref = files.ref
	default:
		return check, fmt.Errorf("未知更新来源 %q", source)
	}
	check.UpdateAvailable = check.LatestVersion > check.CurrentVersion
	return check, nil
}

// ApplyCatalogUpdate 下载并校验新目录后原子落盘（先全部下载到内存校验，
// 再逐文件覆盖写入，最后裁剪已移除的应用目录），并记录目录元数据。
func (s *Service) ApplyCatalogUpdate(ctx context.Context, source string) (domain.CatalogMeta, error) {
	repo := s.repoName()
	var files catalogFiles
	var err error
	switch source {
	case domain.AppCatalogSourceRelease:
		rel, err2 := fetchLatestRelease(ctx, repo)
		if err2 != nil {
			return domain.CatalogMeta{}, err2
		}
		assetURL := findCatalogAsset(rel)
		if assetURL == "" {
			return domain.CatalogMeta{}, fmt.Errorf("release %q 未找到 catalog.zip 资产", rel.TagName)
		}
		files, err = downloadZipFiles(ctx, assetURL)
		if err != nil {
			return domain.CatalogMeta{}, err
		}
		files.ref = rel.TagName
	case domain.AppCatalogSourceMain:
		files, err = fetchBranchCatalog(ctx, repo, "main")
		if err != nil {
			return domain.CatalogMeta{}, err
		}
	default:
		return domain.CatalogMeta{}, fmt.Errorf("未知更新来源 %q", source)
	}

	version, err := catalogVersionOf(files)
	if err != nil {
		return domain.CatalogMeta{}, err
	}
	// 校验全部 app.json（结构完整才允许落盘）。
	ids := make([]string, 0, len(files.apps))
	for id := range files.apps {
		spec, err := loadSpecFromBytes(files.apps[id])
		if err != nil {
			return domain.CatalogMeta{}, fmt.Errorf("校验 %s: %w", id, err)
		}
		if spec.ID != id {
			return domain.CatalogMeta{}, fmt.Errorf("app.json 的 id %q 与目录名 %q 不一致", spec.ID, id)
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)

	// 落盘：覆盖写入全部文件。
	target := filepath.Join(s.appsDir, catalogDirName)
	if err := os.MkdirAll(target, 0o755); err != nil {
		return domain.CatalogMeta{}, err
	}
	write := func(rel string, data []byte) error {
		dst := filepath.Join(target, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dst, data, 0o644)
	}
	if err := write("catalog.json", files.catalog); err != nil {
		return domain.CatalogMeta{}, err
	}
	for _, id := range ids {
		if err := write(filepath.Join(id, "app.json"), files.apps[id]); err != nil {
			return domain.CatalogMeta{}, err
		}
		for name, data := range files.extras[id] {
			if err := write(filepath.Join(id, name), data); err != nil {
				return domain.CatalogMeta{}, err
			}
		}
	}
	// 裁剪：catalog.json 里已不存在的应用目录。
	keepSet := map[string]bool{}
	for _, id := range ids {
		keepSet[id] = true
	}
	if entries, err := os.ReadDir(target); err == nil {
		for _, e := range entries {
			if e.IsDir() && !keepSet[e.Name()] {
				_ = os.RemoveAll(filepath.Join(target, e.Name()))
			}
		}
	}

	meta := domain.CatalogMeta{Source: source, Repo: repo, Version: version, Ref: files.ref, SyncedAt: time.Now()}
	_ = s.repo.SaveCatalogMeta(meta)
	return meta, nil
}

// ── GitHub 数据获取 ──────────────────────────────────────────────────────

type ghRelease struct {
	TagName string `json:"tag_name"`
	Notes   string `json:"body"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

func fetchLatestRelease(ctx context.Context, repo string) (*ghRelease, error) {
	resp, err := ghGet(ctx, githubAPIBase+"/repos/"+repo+"/releases/latest")
	if err != nil {
		return nil, fmt.Errorf("请求 GitHub release 失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("仓库 %s 没有已发布的 release", repo)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API 返回 %d", resp.StatusCode)
	}
	var rel ghRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxFileSize)).Decode(&rel); err != nil {
		return nil, err
	}
	return &rel, nil
}

func findCatalogAsset(rel *ghRelease) string {
	for _, a := range rel.Assets {
		if a.Name == "catalog.zip" || strings.HasSuffix(a.Name, ".catalog.zip") {
			return a.URL
		}
	}
	return ""
}

// catalogFiles 是一次目录下载的产物（内存中，校验通过才落盘）。
type catalogFiles struct {
	ref     string
	catalog []byte
	apps    map[string][]byte            // id → app.json
	extras  map[string]map[string][]byte // id → {guide.md, icon.svg, ...}
}

// downloadZipFiles 下载 catalog.zip 并解出全部文件（带路径安全校验）。
func downloadZipFiles(ctx context.Context, assetURL string) (catalogFiles, error) {
	var out catalogFiles
	resp, err := ghGet(ctx, assetURL)
	if err != nil {
		return out, fmt.Errorf("下载 catalog.zip 失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return out, fmt.Errorf("下载 catalog.zip 返回 %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxAssetSize+1))
	if err != nil {
		return out, err
	}
	if len(raw) > maxAssetSize {
		return out, errors.New("catalog.zip 超过大小上限")
	}
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return out, fmt.Errorf("解析 catalog.zip: %w", err)
	}
	out.apps = map[string][]byte{}
	out.extras = map[string]map[string][]byte{}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := filepath.ToSlash(f.Name)
		if strings.Contains(name, "..") || strings.HasPrefix(name, "/") {
			return out, fmt.Errorf("catalog.zip 内含非法路径 %q", name)
		}
		data, err := readZipFile(f)
		if err != nil {
			return out, err
		}
		if err := out.add(name, data); err != nil {
			return out, err
		}
	}
	if len(out.catalog) == 0 {
		return out, errors.New("catalog.zip 缺少 catalog.json")
	}
	return out, nil
}

func readZipFile(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, maxFileSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxFileSize {
		return nil, fmt.Errorf("文件 %s 超过大小上限", f.Name)
	}
	return data, nil
}

// fetchBranchCatalog 从 main 分支拉目录：catalog.json + 每应用的 app.json /
// guide.md / icon.svg（icon 缺失容忍 404）。路径约定：仓库 apps/catalog/ 下。
func fetchBranchCatalog(ctx context.Context, repo, branch string) (catalogFiles, error) {
	var out catalogFiles
	base := githubRawBase + "/" + repo + "/" + branch + "/apps/catalog"
	getFile := func(rel string, optional bool) ([]byte, error) {
		resp, err := ghGet(ctx, base+"/"+rel)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusNotFound && optional {
			return nil, nil
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("获取 %s 返回 %d", rel, resp.StatusCode)
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, maxFileSize+1))
		if err != nil {
			return nil, err
		}
		if len(data) > maxFileSize {
			return nil, fmt.Errorf("文件 %s 超过大小上限", rel)
		}
		return data, nil
	}
	catalog, err := getFile("catalog.json", false)
	if err != nil {
		return out, err
	}
	var cat domain.Catalog
	if err := json.Unmarshal(catalog, &cat); err != nil {
		return out, fmt.Errorf("解析 catalog.json: %w", err)
	}
	out.ref = branch
	out.catalog = catalog
	out.apps = map[string][]byte{}
	out.extras = map[string]map[string][]byte{}
	for _, ref := range cat.Apps {
		appJSON, err := getFile(ref.ID+"/app.json", false)
		if err != nil {
			return out, err
		}
		out.apps[ref.ID] = appJSON
		out.extras[ref.ID] = map[string][]byte{}
		if guide, err := getFile(ref.ID+"/guide.md", true); err == nil && guide != nil {
			out.extras[ref.ID]["guide.md"] = guide
		}
		if icon, err := getFile(ref.ID+"/"+ref.Icon, true); err == nil && icon != nil && ref.Icon != "" {
			out.extras[ref.ID][ref.Icon] = icon
		}
	}
	return out, nil
}

// add 归档一个文件到 catalogFiles（路径：catalog.json 或 <app>/<file>）。
func (c *catalogFiles) add(name string, data []byte) error {
	switch {
	case name == "catalog.json":
		c.catalog = data
		return nil
	default:
		parts := strings.SplitN(name, "/", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return fmt.Errorf("忽略未知布局文件 %q", name)
		}
		id, file := parts[0], parts[1]
		if file == "app.json" {
			c.apps[id] = data
			return nil
		}
		if c.extras[id] == nil {
			c.extras[id] = map[string][]byte{}
		}
		c.extras[id][file] = data
		return nil
	}
}

// catalogVersionOf 解析 catalog.json 版本并做基本结构检查。
func catalogVersionOf(files catalogFiles) (int, error) {
	if len(files.catalog) == 0 {
		return 0, errors.New("目录缺少 catalog.json")
	}
	var cat domain.Catalog
	if err := json.Unmarshal(files.catalog, &cat); err != nil {
		return 0, fmt.Errorf("解析 catalog.json: %w", err)
	}
	if cat.CatalogVersion <= 0 {
		return 0, errors.New("catalog.json 版本号非法")
	}
	if len(files.apps) == 0 {
		return 0, errors.New("目录不含任何应用")
	}
	return cat.CatalogVersion, nil
}

// loadSpecFromBytes 校验内存中的 app.json。
func loadSpecFromBytes(raw []byte) (domain.AppSpec, error) {
	var spec domain.AppSpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		return domain.AppSpec{}, err
	}
	if err := ValidateSpec(&spec); err != nil {
		return domain.AppSpec{}, err
	}
	return spec, nil
}
