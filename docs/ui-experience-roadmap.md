# UI and experience roadmap

Improve retention among Omarchy users by making doitdoit easier to learn and
comfortable to use daily. Work through these points in separate PRs. Steps 1–4
are implemented; product ideas remain hypotheses until tested with users.

## 1. Make everyday actions visible

- [x] Show Add, Complete, Move, Future, and Help shortcuts in the browsing footer.
- [x] Replace “No tasks” with contextual guidance for adding a task.
- [x] Show confirmation after saved moves and deletions, including the existing
  undo shortcut; confirm undo without offering another undo.

Acceptance: a new user can add, complete, move, and undo without consulting the
README. Automated interaction coverage is in place; validation with new users
remains pending.

## 2. Support tiled windows and long lists

- [x] Adapt the visible column count to terminal width, respecting `-days` as the
  requested maximum.
- [x] Add vertical scrolling that keeps the selected task visible.
- [x] Keep the footer visible and size inputs to their column.
- [x] Add a Today-only view (`T`) and a quick return-to-Today action (`t`).

Acceptance: narrow windows, resizing, long titles, and long lists remain usable
without losing selection or hiding essential controls.

`-days` remains the requested maximum (default 3) and retains its scheduling
window semantics. Resizing only changes presentation. Page Up/Down scroll lists
and oversized titles; short help panels also scroll. Windows below 24×10 pause
task shortcuts and show a resize prompt. Automated coverage verifies these
behaviours; feedback from users remains pending.

## 3. Strengthen visual hierarchy

- [x] Add a selection marker and explicit completion markers.
- [x] Show remaining task counts in day headers.
- [x] Add an optional collapsed completed section without changing stored order.
- [x] Replace hard-coded input colours with theme roles.
- [x] Verify representative light, dark, and custom Omarchy themes.

Acceptance: selection and completion remain distinguishable without colour;
theme changes update the whole interface.

`>` marks the selected task, including its wrapped lines; `[ ]` and `[x]`
indicate completion. `c` folds/unfolds completed tasks for the session. Hidden
tasks cannot receive task actions. Counts and input styles follow the active
theme, including live theme reloads.

## 4. Improve the daily planning experience

- [x] Explain carried-forward work without judgemental messaging.
- [x] Visually separate undated ideas from scheduled tasks within Future.
- [x] Make the existing opt-in live theme hook discoverable during setup.

Acceptance: users understand where tasks went and can plan their day without
changing rollover or storage semantics.

A session notice counts tasks carried to Today at startup, midnight, or an
external reload. Help explains rollover even in small windows. Future groups
Ideas (undated), Scheduled, and Completed without rewriting stored order;
explicit reordering stays within each section. First-run Omarchy setup offers
the existing managed theme hook with a default-No prompt and preserves custom
hooks. Automated coverage verifies these behaviours; user validation remains
pending.

## Delivery and safeguards

- Implement each step in a separate reviewable PR. Start with step 1, then tackle
  responsive layout and visual hierarchy.
- Preserve existing shortcuts, task JSON compatibility, persistence protections,
  and opt-in desktop integration.
- Add behaviour-focused tests beside changed packages. Run Go tests and vet;
  include race tests for persistence, reload, or concurrency changes.
- Test shared lifecycle changes in both Go and the web companion.
- Define detailed interaction choices for each point before implementing it;
  this document is the staged roadmap.
