import { useCallback, useEffect, useState } from "react";
import styles from "../hello.module.css";
import type { AppPageProps } from "../zara";

type NotesResult = { notes: string[] };

export default function Notes({ services }: AppPageProps) {
  const [notes, setNotes] = useState<string[] | null>(null);
  const [draft, setDraft] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const run = useCallback(
    async (name: string, args?: { text: string }) => {
      setBusy(true);
      setError("");
      try {
        const result = (await services.command(name, args)) as NotesResult;
        setNotes(result.notes);
        return true;
      } catch (e) {
        setError((e as Error).message);
        return false;
      } finally {
        setBusy(false);
      }
    },
    [services]
  );

  useEffect(() => {
    void run("list_notes");
  }, [run]);

  return (
    <div className={styles.page} data-testid="hello-notes">
      <h1 className={styles.title}>Notes</h1>
      <p className={styles.subtitle}>Stored by the app's Python backend through commands.</p>
      <form
        className={styles.row}
        onSubmit={async (event) => {
          event.preventDefault();
          if (await run("add_note", { text: draft })) setDraft("");
        }}
      >
        <input
          className={styles.input}
          aria-label="New note"
          value={draft}
          onChange={(event) => setDraft(event.target.value)}
          placeholder="Write a note"
        />
        <button className={styles.button} type="submit" disabled={busy}>
          Add
        </button>
        <button className={styles.button} type="button" disabled={busy} onClick={() => void run("clear_notes")}>
          Clear
        </button>
      </form>
      {error && (
        <p className={styles.error} data-testid="hello-notes-error">
          {error}
        </p>
      )}
      {notes === null ? (
        <p>Loading…</p>
      ) : notes.length === 0 ? (
        <p data-testid="hello-notes-empty">No notes yet.</p>
      ) : (
        <ul className={styles.list} data-testid="hello-notes-list">
          {notes.map((note, index) => (
            <li key={index}>{note}</li>
          ))}
        </ul>
      )}
    </div>
  );
}
