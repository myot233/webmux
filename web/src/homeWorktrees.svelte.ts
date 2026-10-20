import { stored, store } from "./api";
import { HomeWorktreesSchema, type HomeWorktrees } from "./schemas";
import { parseHomeWorktrees } from "./homeWorktrees";

const key = "homeWorktrees";

class HomeWorktreesStore {
  value = $state.raw<HomeWorktrees>(parseHomeWorktrees(stored(key)));
  save(next: HomeWorktrees) {
    if (next === this.value) return;
    this.value = HomeWorktreesSchema.parse(next);
    store(key, JSON.stringify(this.value));
  }
}

export const homeWorktrees = new HomeWorktreesStore();

window.addEventListener("storage", (event) => {
  if (event.key === null || event.key === `webmux.${key}`)
    homeWorktrees.value = parseHomeWorktrees(stored(key));
});
