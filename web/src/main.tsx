import React from "react";
import { createRoot } from "react-dom/client";
import "./ui.css";
import "./style.css";
import "./workspace.css";
import "./public-site.css";
import "./workspace-polish.css";
import Workspace from "./workspace";
createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <Workspace />
  </React.StrictMode>,
);
