import { FolderTree } from "lucide-react";

import { PlaceholderView } from "@/components/layout/PlaceholderView";

/**
 * SFTP file workspace view. Phase 1: placeholder.
 * Phase 3 will add the remote file explorer with upload/download/rename/delete.
 */
export function SftpView() {
  return (
    <PlaceholderView
      icon={FolderTree}
      title="File Workspace"
      description="Browse, upload, download, rename, and delete remote files over SFTP."
      phase="Phase 3 — File Management"
    />
  );
}
