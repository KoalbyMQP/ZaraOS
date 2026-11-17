import { lazy, Suspense, useEffect, useState } from "react";
import robot from "../assets/robot.svg";
import styles from "../hello.module.css";
import type { AppPageProps, JsonValue } from "../zara";

// Loaded as a separate chunk, so a page exercises chunk loading from the robot.
const LiveValues = lazy(() => import("../components/LiveValues"));

export default function Overview({ instanceId, services }: AppPageProps) {
  const [live, setLive] = useState<Record<string, JsonValue>>({});
  const [streamError, setStreamError] = useState("");
  const [requested, setRequested] = useState<number | null>(null);
  const [controlError, setControlError] = useState("");

  useEffect(
    () =>
      services.telemetry((values) => setLive((current) => ({ ...current, ...values })), {
        maxHz: 20,
        onError: setStreamError,
      }),
    [services]
  );

  const applied = live["controls.wave_hz"];

  return (
    <div className={styles.page} data-testid="hello-overview">
      <header className={styles.header}>
        <img className={styles.logo} src={robot} alt="" data-testid="hello-logo" />
        <div>
          <h1 className={styles.title}>Hello UI</h1>
          <p className={styles.subtitle}>
            Instance <span className={styles.value}>{instanceId}</span>
          </p>
        </div>
      </header>

      <section className={styles.card}>
        <h2 className={styles.cardTitle}>Simulated sensor</h2>
        <div className={styles.row}>
          <label htmlFor="hello-wave">Wave frequency</label>
          <input
            id="hello-wave"
            type="range"
            min={0.2}
            max={5}
            step={0.2}
            defaultValue={1}
            onChange={(event) => {
              const value = Number(event.target.value);
              setRequested(value);
              services.setControl("wave_hz", value).catch((error: Error) => setControlError(error.message));
            }}
          />
        </div>
        <p data-testid="hello-requested">
          Requested: <span className={styles.value}>{requested ?? "-"}</span> Hz · Applied:{" "}
          <span className={styles.value} data-testid="hello-applied">
            {applied == null ? "-" : String(applied)}
          </span>{" "}
          Hz
        </p>
        {controlError && <p className={styles.error}>{controlError}</p>}
      </section>

      <Suspense fallback={<p>Loading live values…</p>}>
        <LiveValues values={live} />
      </Suspense>
      {streamError && (
        <p className={styles.error} data-testid="hello-stream-error">
          Telemetry: {streamError}
        </p>
      )}
    </div>
  );
}
