// Entry point: mount the app into the page the master serves.

import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import "./styles.css";
import { App } from "./App.jsx";

const root = createRoot(document.getElementById("root"));
root.render(
  <StrictMode>
    <App />
  </StrictMode>,
);
