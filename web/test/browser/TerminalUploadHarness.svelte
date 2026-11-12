<script lang="ts">
  import { untrack } from "svelte";
  import {
    useUpload,
    type UploadInput,
  } from "../../src/hooks/shared/upload.svelte";
  import type { Connection } from "../../src/platform/ws";

  let {
    id = "sess-upload",
    initialConnection = "connected",
    initialExitCode = null,
    initialRestoring = false,
    focus,
    input,
    setError,
    oninit,
  }: {
    id?: string;
    initialConnection?: Connection;
    initialExitCode?: number | null;
    initialRestoring?: boolean;
    focus: () => void;
    input: UploadInput;
    setError: (message: string) => void;
    oninit: (api: {
      upload: ReturnType<typeof useUpload>;
      host: HTMLElement;
      setConnection: (c: Connection) => void;
      setExitCode: (code: number | null) => void;
      setRestoring: (r: boolean) => void;
    }) => void;
  } = $props();

  let hostEl = $state<HTMLElement | null>(null);
  let connection = $state<Connection>(untrack(() => initialConnection));
  let exitCode = $state<number | null>(untrack(() => initialExitCode));
  let restoring = $state<boolean>(untrack(() => initialRestoring));

  const terminal = {
    get exitCode() {
      return exitCode;
    },
    get connection() {
      return connection;
    },
    get restoring() {
      return restoring;
    },
    focus: () => focus(),
  };

  const upload = untrack(() =>
    useUpload(() => hostEl!, id, terminal, setError, input),
  );

  $effect(() => {
    if (hostEl) {
      oninit({
        upload,
        host: hostEl,
        setConnection: (c) => {
          connection = c;
        },
        setExitCode: (code) => {
          exitCode = code;
        },
        setRestoring: (r) => {
          restoring = r;
        },
      });
    }
  });
</script>

<div bind:this={hostEl} data-testid="upload-host"></div>
