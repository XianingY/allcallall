import React from "react";
import ReactDOM from "react-dom/client";
import { BrowserRouter } from "react-router-dom";

import { App } from "@/app/App";
import { AppProviders } from "@/app/AppProviders";
import { AppErrorBoundary } from "@/components/AppErrorBoundary";
import "@/i18n";
import "@/styles.css";

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <AppErrorBoundary>
      <AppProviders>
        <BrowserRouter>
          <App />
        </BrowserRouter>
      </AppProviders>
    </AppErrorBoundary>
  </React.StrictMode>,
);
