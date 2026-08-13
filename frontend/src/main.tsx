import React from "react";
import ReactDOM from "react-dom/client";

import "@/styles/globals.css";
import "@/i18n"; // initialise i18next before the app renders
import { AppShell } from "@/components/layout/AppShell";
import { QueryProvider } from "@/app/providers/QueryProvider";

ReactDOM.createRoot(
  document.getElementById("root") as HTMLElement,
).render(
  <React.StrictMode>
    <QueryProvider>
      <AppShell />
    </QueryProvider>
  </React.StrictMode>,
);
