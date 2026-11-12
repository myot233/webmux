import { settings, changeSettings } from "../settings/settings.svelte";
import {
  HomeWorktreesSchema,
  type HomeWorktrees,
} from "../../contracts/schemas";

export const homeWorktrees = {
  get value(): HomeWorktrees {
    return settings.values.homeWorktrees ?? {};
  },
  save(next: HomeWorktrees) {
    changeSettings({ homeWorktrees: HomeWorktreesSchema.parse(next) });
  },
};
