import { mount } from "svelte";
import App from "./App.svelte";
import { initializeTheme } from "./theme";
import { trackInstallation } from "./install.svelte";
import "./styles.css";

initializeTheme();
trackInstallation();
mount(App, { target: document.getElementById("app")! });
