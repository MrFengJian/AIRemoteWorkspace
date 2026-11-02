// Package appdeploy 实现应用部署与管理：内置应用目录（app.json 操作手册
// + guide.md 指南）随二进制分发，启动时种子解压到数据目录；部署/监控/控制
// 按 app.json 定义的命令经 SSH 执行，安装状态持久化在本地数据库。
//
// 规范（schema、status 输出契约、catalog.zip 发布约定）见 docs/APP_CATALOG.md。
package appdeploy

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ai-remote/workspace/internal/domain"
)

// builtinCatalog 是随二进制分发的内置应用目录（catalog.json + 每应用目录）。
//
//go:embed all:catalog
var builtinCatalog embed.FS

// catalogDirName / customDirName 是数据目录 apps/ 下的两个固定子目录：
// catalog 存内置（或经 GitHub 更新的）目录，custom 存用户自建应用——种子与
// 更新绝不触碰 custom。
const (
	catalogDirName = "catalog"
	customDirName  = "custom"
)

// lastSeedOverwrite 记录最近一次 SeedBuiltinCatalog 是否执行了覆盖写入；
// seedMu 同时守护它与覆盖判定的读写。
var (
	seedMu            sync.Mutex
	lastSeedOverwrite bool
)

// SeedBuiltinCatalog 在启动时把内置目录种子解压到 <appsDir>/catalog。覆盖规则：
//   - 内置 catalogVersion 大于磁盘版本 → 覆盖（正常版本升级路径）；
//   - 版本相同或磁盘无版本，但磁盘来源为内置（或来源未知，如旧版开发数据）
//     且内容哈希与内置不一致 → 覆盖（开发期改目录立即生效，无需手改版本号）；
//   - 磁盘来源为 GitHub（release/main）→ 永不覆盖，避免旧二进制降级在线目录。
//
// 用户自建应用在 custom/ 下不受影响；部分文件失败不阻塞启动。

// LastSeedWasOverwriteReset 清零覆盖标记（测试用）。
func LastSeedWasOverwriteReset() {
	seedMu.Lock()
	lastSeedOverwrite = false
	seedMu.Unlock()
}

// LastSeedWasOverwrite 报告最近一次 SeedBuiltinCatalog 是否执行了覆盖写入。
func LastSeedWasOverwrite() bool {
	seedMu.Lock()
	defer seedMu.Unlock()
	return lastSeedOverwrite
}

func SeedBuiltinCatalog(appsDir string, diskSource string) error {
	seedMu.Lock()
	lastSeedOverwrite = false
	seedMu.Unlock()
	builtinVersion, err := readBuiltinVersion()
	if err != nil {
		return err // 内嵌目录损坏属于打包错误，直接暴露
	}
	catalogDir := filepath.Join(appsDir, catalogDirName)
	diskVersion := readDiskCatalogVersion(catalogDir)
	if builtinVersion > diskVersion {
		markSeedOverwrite()
		return copyBuiltinTo(appsDir)
	}
	if diskSource != domain.AppCatalogSourceRelease && diskSource != domain.AppCatalogSourceMain {
		// 内置来源（或来源未知的开发数据）：同版本也做内容比对，漂移即覆盖。
		builtinHash, err := builtinTreeHash()
		if err != nil {
			return err
		}
		if diskTreeHash(catalogDir) != builtinHash {
			markSeedOverwrite()
			return copyBuiltinTo(appsDir)
		}
	}
	return nil // 磁盘与内置一致，或来自 GitHub 且不落后——不覆盖
}

func markSeedOverwrite() {
	seedMu.Lock()
	lastSeedOverwrite = true
	seedMu.Unlock()
}

// builtinTreeHash 对内嵌 catalog/ 全树（相对路径 + 内容）计算确定性哈希。
func builtinTreeHash() (string, error) {
	h := sha256.New()
	err := fs.WalkDir(builtinCatalog, "catalog", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		raw, err := builtinCatalog.ReadFile(path)
		if err != nil {
			return err
		}
		h.Write([]byte(path))
		h.Write([]byte{0})
		h.Write(raw)
		h.Write([]byte{0})
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// diskTreeHash 与 builtinTreeHash 同规则地哈希磁盘 catalog/ 目录（目录不存在
// 返回空串，必然与内置不一致）。
func diskTreeHash(catalogDir string) string {
	h := sha256.New()
	_ = fs.WalkDir(os.DirFS(catalogDir), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		raw, err := os.ReadFile(filepath.Join(catalogDir, filepath.FromSlash(path)))
		if err != nil {
			return nil
		}
		h.Write([]byte("catalog/" + path))
		h.Write([]byte{0})
		h.Write(raw)
		h.Write([]byte{0})
		return nil
	})
	return hex.EncodeToString(h.Sum(nil))
}

// readBuiltinVersion 解析内嵌 catalog.json 的版本。
func readBuiltinVersion() (int, error) {
	raw, err := builtinCatalog.ReadFile("catalog/catalog.json")
	if err != nil {
		return 0, err
	}
	var cat domain.Catalog
	if err := json.Unmarshal(raw, &cat); err != nil {
		return 0, err
	}
	return cat.CatalogVersion, nil
}

// readDiskCatalogVersion 读取磁盘 catalog.json 版本；缺失/损坏 = 0（触发种子）。
func readDiskCatalogVersion(catalogDir string) int {
	raw, err := os.ReadFile(filepath.Join(catalogDir, "catalog.json"))
	if err != nil {
		return 0
	}
	var cat domain.Catalog
	if json.Unmarshal(raw, &cat) != nil {
		return 0
	}
	return cat.CatalogVersion
}

// copyBuiltinTo 把内嵌 catalog/ 全量写入 <appsDir>/catalog（覆盖同名文件），
// 并删除内嵌目录中已不存在的应用目录（版本升级裁剪）。
func copyBuiltinTo(appsDir string) error {
	target := filepath.Join(appsDir, catalogDirName)
	keep := map[string]bool{}
	err := fs.WalkDir(builtinCatalog, "catalog", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel := strings.TrimPrefix(path, "catalog/")
		if rel == "" {
			return nil
		}
		keep[strings.SplitN(rel, "/", 2)[0]] = true
		raw, err := builtinCatalog.ReadFile(path)
		if err != nil {
			return nil // 单文件失败不阻塞种子
		}
		dst := filepath.Join(target, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return nil
		}
		_ = os.WriteFile(dst, raw, 0o644)
		return nil
	})
	if err != nil {
		return err
	}
	// 裁剪：磁盘上存在但内置已移除的应用目录（仅限 catalog/，不动 custom/）。
	entries, err := os.ReadDir(target)
	if err != nil {
		return nil // 目录不存在等——下次启动重试
	}
	for _, e := range entries {
		if !e.IsDir() || keep[e.Name()] {
			continue
		}
		_ = os.RemoveAll(filepath.Join(target, e.Name()))
	}
	return nil
}

// loadSpecFromDir 读取并解析一个应用目录的 app.json。
func loadSpecFromDir(dir string) (domain.AppSpec, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "app.json"))
	if err != nil {
		return domain.AppSpec{}, err
	}
	var spec domain.AppSpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		return domain.AppSpec{}, err
	}
	if err := ValidateSpec(&spec); err != nil {
		return domain.AppSpec{}, err
	}
	return spec, nil
}

// listSpecDirs 枚举 base 下含 app.json 的应用目录（按目录名排序）。
func listSpecDirs(base string) ([]string, error) {
	entries, err := os.ReadDir(base)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var dirs []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(base, e.Name(), "app.json")); err == nil {
			dirs = append(dirs, e.Name())
		}
	}
	return dirs, nil
}
