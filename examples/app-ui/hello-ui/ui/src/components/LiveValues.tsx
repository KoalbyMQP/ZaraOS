import styles from "../hello.module.css";
import type { JsonValue } from "../zara";

/** Telemetry readout. Lives in its own lazily loaded chunk (see pages/Overview.tsx). */
export default function LiveValues({ values }: { values: Record<string, JsonValue> }) {
  const wave = typeof values.wave === "number" ? values.wave : 0;
  return (
    <section className={styles.card} data-testid="hello-live">
      <h2 className={styles.cardTitle}>Live values</h2>
      <p>
        Uptime:{" "}
        <span className={styles.value} data-testid="hello-uptime">
          {String(values.uptime_s ?? "-")}
        </span>{" "}
        s · Notes: <span className={styles.value}>{String(values.notes ?? "-")}</span>
      </p>
      <p>
        Wave: <span className={styles.value}>{wave.toFixed(2)}</span>
      </p>
      <div className={styles.bar}>
        <div className={styles.barFill} style={{ width: `${((wave + 1) / 2) * 100}%` }} />
      </div>
    </section>
  );
}
