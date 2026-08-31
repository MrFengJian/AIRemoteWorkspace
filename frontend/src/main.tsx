import React from "react";
import ReactDOM from "react-dom/client";

import "@/styles/globals.css";
import "@/i18n"; // initialise i18next before the app renders
import { AppShell } from "@/components/layout/AppShell";
import { QueryProvider } from "@/app/providers/QueryProvider";
import { ThemeProvider } from "@/app/providers/ThemeProvider";
import { SftpWindowApp } from "@/features/sftp/SftpWindowApp";

/**
 * Window routing: the standalone SFTP window (opened per host from the hosts
 * sidebar) loads this same bundle with "#/sftp-window?host=<id>" — render
 * the SFTP workbench alone there; the main window keeps the full app shell.
 */
function sftpWindowHostFromHash(): string | null {
  const m = /^#\/sftp-window\?host=([^&]+)$/.exec(window.location.hash);
  return m ? decodeURIComponent(m[1]) : null;
}

const sftpHostID = sftpWindowHostFromHash();

ReactDOM.createRoot(
  document.getElementById("root") as HTMLElement,
).render(
  <React.StrictMode>
    <ThemeProvider>
      <QueryProvider>
        {sftpHostID ? (
          <SftpWindowApp hostID={sftpHostID} />
        ) : (
          <AppShell />
        )}
      </QueryProvider>
    </ThemeProvider>
  </React.StrictMode>,
);
