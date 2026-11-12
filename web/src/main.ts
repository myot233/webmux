import { mount } from "svelte";
import App from "./App.svelte";
import { initializeTheme } from "./hooks/settings/theme";
import { trackInstallation } from "./platform/install.svelte";
import "./styles.css";

initializeTheme();
trackInstallation();
mount(App, { target: document.getElementById("app")! });
