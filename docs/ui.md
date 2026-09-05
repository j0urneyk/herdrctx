# UI behavior

Use these rules when changing action feedback, forms, or modal input handling. User-facing keyboard shortcuts are documented in [README.md](../README.md#keyboard-shortcuts) and [README.ko.md](../README.ko.md#단축키).

## Alerts

Use alert dialogs for one-shot, user-triggered warnings and errors that explain why the requested action did not run or failed. These include:

- Nested Herdr attach/create blocks.
- No selected session for an action.
- Default-session delete attempts.
- Running-session delete attempts.
- Attach, stop, or delete command failures.

Dialogs are alerts, not confirmations. Stopping or deleting a session still requires a separate confirmation before the command runs.

Close alerts with `enter`, `esc`, or `q`. Keep `ctrl+c` as app quit. While an alert is open, the background table, form, and confirmation UI must not receive key input. Prefer overlay rendering so the session list remains visible behind the alert.

Nested popovers receive key input before their parent modal; `esc` closes one layer at a time.

## Status and validation

Keep automatic refresh/list failures in the status line. Manual refresh failures use the same list code path and also stay in the status line.

Keep form validation errors near the relevant input so the user can correct them in place.
