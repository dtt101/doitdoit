# UI and experience roadmap

Improve retention among Omarchy users by making doitdoit easier to learn and
comfortable to use daily. Work through these points in separate PRs. Steps 1–2
are implemented; product ideas remain hypotheses until tested with users.

## 1. Make everyday actions visible

- [x] Show Add, Complete, Move, Future, and Help shortcuts in the browsing footer.
- [x] Replace “No tasks” with contextual guidance for adding a task.
- [x] Show confirmation after saved moves and deletions, including the existing
  undo shortcut; confirm undo without offering another undo.
- [ ] Consider a searchable command menu only if user feedback establishes a need.

Acceptance: a new user can add, complete, move, and undo without consulting the
README. Automated interaction coverage is in place; validation with new users
remains part of step 6.

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

- [ ] Add a selection marker and explicit completion markers.
- [ ] Show remaining task counts in day headers.
- [ ] Add an optional collapsed completed section without changing stored order.
- [ ] Replace hard-coded input colours with theme roles.
- [ ] Verify representative light, dark, and custom Omarchy themes.

Acceptance: selection and completion remain distinguishable without colour;
theme changes update the whole interface.

## 4. Improve the daily planning experience

- [ ] Test whether accumulated rollover tasks contribute to abandonment.
- [ ] If supported by feedback, introduce an optional morning review using
  existing keep, move, and Future actions.
- [ ] Explain carried-forward work without judgemental messaging.
- [ ] Visually separate undated ideas from scheduled tasks within Future.

Acceptance: users understand where tasks went and can plan their day without
changing rollover or storage semantics.

## 5. Improve Omarchy integration

- [ ] Add an optional launcher capture flow backed by `doitdoit add`.
- [ ] Let users choose the shortcut and destination.
- [ ] Confirm successful capture and return focus to the previous workflow.
- [ ] Make the existing opt-in live theme hook discoverable during setup.

Acceptance: capture works without opening the full planner; integration preserves
existing user shortcuts and hooks.

## 6. Validate with former users

- [ ] Recruit five people who tried doitdoit and stopped.
- [ ] Observe adding, completing, rescheduling, and undoing a deletion.
- [ ] Ask what replaced doitdoit and why.
- [ ] Repeat those tasks with the improved version.
- [ ] Follow up after one week about continued use and remaining friction.

Use manual feedback; retain the no-telemetry policy.

## Delivery and safeguards

- Implement each step in a separate reviewable PR. Start with step 1, then tackle
  responsive layout and visual hierarchy.
- Begin user research before later implementation; use findings to prioritise
  steps 4–5.
- Preserve existing shortcuts, task JSON compatibility, persistence protections,
  and opt-in desktop integration.
- Add behaviour-focused tests beside changed packages. Run Go tests and vet;
  include race tests for persistence, reload, or concurrency changes.
- Test shared lifecycle changes in both Go and the web companion.
- Define detailed interaction choices for each point before implementing it;
  this document is the staged roadmap.
