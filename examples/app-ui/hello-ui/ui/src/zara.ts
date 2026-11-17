// The page contract from the Driver Station (apps/driver-station/lib/app-ui/types.ts).
// Copied here until it is published as a package; keep in sync.

export type JsonValue = string | number | boolean | null | JsonValue[] | { [key: string]: JsonValue };

export type AppCommand = {
  name: string;
  doc: string | null;
  params: { name: string; required: boolean; type?: string; default?: JsonValue }[];
};

export type AppControl = {
  name: string;
  type: string | null;
  min: number | null;
  max: number | null;
  step: number | null;
  default: JsonValue;
  value: JsonValue;
  doc: string | null;
};

type RequestOptions = { signal?: AbortSignal };

export type AppServices = {
  command: (name: string, args?: Record<string, JsonValue>, options?: RequestOptions) => Promise<unknown>;
  commands: (options?: RequestOptions) => Promise<AppCommand[]>;
  controls: (options?: RequestOptions) => Promise<AppControl[]>;
  setControl: (
    name: string,
    value: JsonValue,
    options?: RequestOptions & { seq?: number }
  ) => Promise<{ accepted: boolean; stale: boolean; value: JsonValue }>;
  telemetry: (
    onValues: (values: Record<string, JsonValue>) => void,
    options?: RequestOptions & { maxHz?: number; onError?: (message: string) => void }
  ) => () => void;
};

export type AppPageProps = {
  instanceId: string;
  services: AppServices;
};
