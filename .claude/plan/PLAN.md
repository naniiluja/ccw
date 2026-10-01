# Implementation Plan — ccw

> **Execution rule: VERTICAL SLICES, RUN IN WAVES.** `/ccf:cook` runs tasks with no link between them (no `Depends on`, no shared file, no shared hotspot) at the same time, each in its own worktree.
> Each task is a thin tracer-bullet that crosses all the layers it touches (DB + service + UI), NOT a horizontal "all-DB-then-all-API" phase, so integration is proven early.
> A task starts only after every task in its `Depends on` has a **GREEN gate** and is merged; each wave's merged result is tested before the next wave starts.
> The `in-progress`/`in-review` status is read by the session-start hook to re-load context after compact, keep status up to date.

## Milestones
- M2: tách repo thành `be/` và `fe/`, dựng dashboard cho toàn bộ chức năng của ccw bằng shadcn và Origin UI (không chế component), phục vụ dưới `/ui/` từ binary Go.

> Status: `todo` / `in-progress` / `in-review` / `done` / `blocked`. Lifecycle: `todo → in-progress → in-review → done`. A task becomes `in-review` once its code+test are complete and merged (by `/ccf:cook`, or implemented directly in the session); only `/ccf:updatespec` writes `done` after `/ccf:check` passes.
> Write the status as a **bare word**, no `**bold**` around it. Emphasis carries no information and the status is matched as a whole word.
> Per-task detail in `task-NNN-*.md`.

> **Keep this file to the CURRENT iteration.** When every task of an iteration is `done`, `/ccf:updatespec` moves its sections verbatim into `.claude/plan/ARCHIVE.md` and `git mv`s its `task-NNN-*.md` files into `.claude/plan/archive/`. A closed row left here is counted as live work by the session-start and Stop hooks, while DELETING the history would lose a real record of what shipped and why. So archive, never delete.

<!-- Keep the status guidance ABOVE this line, never below "## Origin": archive retirement groups content from one "## Origin" heading to the next, and only text before the FIRST heading is never cut into ARCHIVE.md. -->
