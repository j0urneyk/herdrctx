# UI behavior

Use this guide when implementing or reviewing session actions, forms, feedback, and keyboard routing. [AGENTS.md](../AGENTS.md) defines the mandatory project rules. The [English](../README.md#usage) and [Korean](../README.ko.md#사용법) READMEs are the user guides; keep both aligned when behavior changes. Choose verification from the [testing guide](testing.md#choosing-checks).

## Session list and refresh

The list loads at startup and refreshes every `3s` by default. The CLI accepts a different `--interval`, with a minimum of `500ms`. Automatic ticks start a request only when no list request or session action is in progress. Manual refresh (`r`) also avoids overlapping list requests. A dialog blocks background key input, but does not pause refresh messages.

Keep the previous list on refresh failure and put the error in the status line. Successful refreshes preserve the selected session by name when it remains visible; otherwise the cursor stays within the remaining rows. Ignore stale responses from earlier refresh requests. After the initial loading screen, subsequent refreshes show a refreshing indicator only after `300ms`, avoiding flicker on quick refreshes.

Search filters the displayed list by a case-insensitive substring of the session name or Herdr's `session_dir` field. The **Directory** column and directory search use that session-state path, not the working directory entered when creating a session. Opening search with `/` starts editing; `tab` switches scope without clearing the query, `enter` keeps the filter, and `esc` clears it. Filters survive refreshes, and actions target the selected filtered row. Distinguish an empty session list from a filter with no matches.

## Attach and create

Keep Herdr as the external CLI boundary. Attach uses `herdr session attach <name>`. Creation uses `herdr --session <name>` with the child process's working directory set to the selected start directory. Both use Bubble Tea's `tea.ExecProcess` to hand the terminal to Herdr immediately. The TUI resumes after Herdr returns, normally when the user detaches, and refreshes the session list. Do not replace this with background creation or a second attach step. Herdr owns whether the named session is created or an existing one is reused.

Block attach and both creation shortcuts before launching Herdr when `HERDR_ENV=1` or a nonempty `HERDR_SOCKET_PATH` identifies a nested context. The supported opt-ins are `--allow-nested` and `HERDRCTX_ALLOW_NESTED=1`; Herdr also needs its own [`experimental.allow_nested`](https://herdr.dev/docs/config-reference/#experimental) setting. Show a warning dialog when blocked, including when attaching to the session that already contains the TUI. This guard does not disable list, stop, or delete actions.

### Creation forms

`n` asks for a name and uses the model's default directory, normally the directory where `herdrctx` started. That directory must exist. `N` adds an editable start-directory field, initially filled with the default directory. It rejects empty or whitespace-only input and creates missing directories before launching Herdr. A path that names a file, contains control characters, or cannot be accessed or created produces a form error.

Relative paths resolve against the default directory. `~` and `~/...` expand to the user's home; paths are not evaluated by a shell. Preserve meaningful spaces in nonempty paths. Directories created by the `N` form are not rolled back if the subsequent Herdr launch fails.

New names must be 1–64 ASCII characters, begin with a letter or number, and otherwise contain only letters, numbers, `-`, `_`, or `.`. `help` is reserved. Reject invalid names before creating directories or invoking Herdr. Keep validation errors near the relevant input and focus the field that needs correction. Command-construction errors also remain in the form; failures after terminal handoff use the attach-error dialog when the TUI resumes.

Existing names have a separate validation contract: names such as `.hidden`, `_scratch`, or `-old` may still appear in the list. Do not apply the stricter creation rule to all loaded sessions. Herdr treats `help`, `-h`, and `--help` as help arguments during attach, and `--json` as a flag during stop/delete, so those action-specific cases are blocked with warnings.

### Directory completion

With the `N` directory field focused, editing the path offers matching directories, including symlinks to directories. Missing or unreadable parent directories produce no suggestions; submission still performs path validation. Completion is bounded to scanning `4096` entries and collecting `256` matches, so it is not an exhaustive directory listing.

The default visible window is eight rows. `--complete-count` (or `HERDRCTX_COMPLETE_COUNT`) accepts a positive integer and changes the visible window, not the match limit. `--complete-hidden` (or `HERDRCTX_COMPLETE_HIDDEN`) accepts `auto`, `always`, or `never`. In `auto`, hidden directories appear when the current path segment starts with `.`. Flags take precedence over environment defaults.

While suggestions are open, `↑` / `↓` and `ctrl+p` / `ctrl+n` cycle through matches, including those outside the visible window. `tab` accepts a match and closes suggestions; it does not submit the form. Further path edits may reopen suggestions. `enter` submits the current input even when suggestions are open. With suggestions closed, `↑` / `↓` switch fields and `tab` does not switch fields. `esc` closes suggestions first; a second `esc` cancels the form.

## Stop and delete

Both actions require confirmation before the command runs. Stopping can terminate panes, shells, agents, servers, tests, and other processes. Deleting removes saved session state and cannot be undone. Keep these consequences in the confirmation itself.

For stop, require a selected running session. For delete, require a selected, stopped, non-default session. Both actions reject the name `--json`, which Herdr interprets as a flag. Block default-session deletion before invoking Herdr. A running session must be stopped as a separate confirmed action; never combine stop and delete.

The confirmation captures the session's name, so a refresh or cursor change cannot silently choose another target. On `y` or `enter`, recheck that target against the current model list. If it disappeared or no longer meets the conditions, show a warning and do not run the action. `n` or `esc` cancels.

Deletion also fetches `herdr session list --json` immediately before invoking `herdr session delete <name> --json`. If this fetch fails or the target is missing, default, or running, fail the action without invoking delete. This is an additional check, not an atomic lock on Herdr state. Stop invokes `herdr session stop <name> --json` after model revalidation.

After either command succeeds or fails, clear the busy state and refresh the list. While busy, allow list movement, help, and quit, but block new session actions, search, and manual refresh.

## Feedback and input ownership

Choose feedback by the operation that failed:

| Situation | Where it appears |
| --- | --- |
| A user action is blocked: nested attach/create, no selection, unavailable target, already-stopped session, protected delete, or an action-specific name restriction | Warning dialog |
| Attach/create fails after handoff, or a confirmed stop/delete action fails, including its delete preflight | Error dialog |
| Initial list load, automatic refresh, or manual refresh fails | Status line |
| Name or directory validation fails while creating a session | Creation form, with the relevant field focused |
| Loading, action progress, successful completion, or return from Herdr | Status or summary line |

Alerts explain a warning or error; they never authorize an action. A stop/delete confirmation remains a separate state. An automatic refresh may update status behind an alert, but the alert stays open until dismissed.

Route `ctrl+c` first so it quits from every TUI layer and cancels the model's command context. While Herdr owns the terminal, Herdr handles its input. Nested popovers receive keys before their parent modal, and `esc` closes one layer at a time. For other keys, the active layer owns input as follows:

| Active layer | Key behavior |
| --- | --- |
| Alert dialog | `enter`, `esc`, or `q` closes it. Consume all other keys; do not send them to a form, confirmation, or table. |
| Directory suggestions inside a creation form | Handle completion keys before the parent form. Pass ordinary text and `enter` to the form. |
| Creation form | `enter` submits and `esc` cancels after any suggestions close. Printable keys such as `q` are input, not global shortcuts. |
| Search field | Handle search keys and text only; `q` is search input. |
| Stop/delete confirmation | `y` / `enter` confirms, `n` / `esc` cancels, and `q` quits the app. Other keys do not reach the table. |
| Busy session action | `q` quits; movement and help remain available. Other action keys are consumed. |
| Session list | Use the [documented shortcuts](../README.md#keyboard-shortcuts). |

Render alerts, confirmations, and creation forms as overlays with the session list visible behind them. Hide the root status/help footer while an overlay is visible, leaving its own instructions visible. Sanitize external names, paths, and error text for terminal display without changing the raw values used for commands and selection.

## Where to change and verify behavior

[model.go](../internal/ui/model.go) owns action state, refresh messages, and overlays; [input_layer.go](../internal/ui/input_layer.go) and [keymap.go](../internal/ui/keymap.go) own keyboard routing. Forms and suggestions live in [new_session.go](../internal/ui/new_session.go) and [path_completion.go](../internal/ui/path_completion.go). The [Herdr client](../internal/herdr/client.go), [name validation](../internal/herdr/types.go), [directory validation](../internal/herdr/path.go), and [environment detection](../internal/herdr/env.go) define the integration boundary.

Extend the relevant cases in [model_test.go](../internal/ui/model_test.go), [path_completion_test.go](../internal/ui/path_completion_test.go), and the Herdr package tests when behavior changes. Check blocked actions, cancellation, stale session state, and input isolation as well as successful actions. Use [Choosing checks](testing.md#choosing-checks) for the required commands; the real lifecycle test does not cover every form, alert, or terminal layout.
