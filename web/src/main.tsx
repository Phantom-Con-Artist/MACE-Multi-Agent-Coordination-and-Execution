import React from "react";
import { createRoot } from "react-dom/client";
import "./ui.css";
import "./style.css";
import "./workspace.css";
import "./public-site.css";
import "./workspace-polish.css";
import "./theme.css";
import Workspace from "./workspace";
createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <Workspace />
  </React.StrictMode>,
);

// The service worker makes MACE installable and carries notifications. It
// needs a secure context: HTTPS (mace-server start) or localhost.
if ("serviceWorker" in navigator && window.isSecureContext)
  void navigator.serviceWorker.register("/sw.js").catch(() => {});
