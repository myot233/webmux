<script lang="ts">
  import { Button } from "$lib/components/ui/button";
  import UsageRing from "../src/components/chat/UsageRing.svelte";
  let used = $state(40_000);
</script>

<section class="mx-auto flex max-w-sm flex-col gap-4 p-6">
  <h1 class="text-base font-medium">上下文用量</h1>
  <p class="text-sm text-muted-foreground">
    演示 agent 用量报告；数字立即更新，图形平滑跟随。
  </p>
  <div>
    <UsageRing usage={{ used, size: 200_000, cost: 0.07, currency: "USD" }} />
  </div>
  <div class="flex flex-wrap gap-2">
    <Button variant="outline" onclick={() => (used = 160_000)}>报告 80%</Button>
    <Button variant="outline" onclick={() => (used = 40_000)}>报告 20%</Button>
  </div>
</section>
