# Frontend Quality Contract

This is a SpeQ-inspired contract for a frontend agent. It is intentionally plain markdown so any agent can read and follow it without a custom parser.

## Vocabulary

Use these terms consistently:

| Term | Meaning |
|---|---|
| Surface | A page, modal, panel, app view, or self-contained UI area |
| Workflow | The user path the surface exists to support |
| Primary action | The next action the target user is most likely to take |
| State | Loading, empty, error, disabled, success, selected, active, expanded, or persisted UI condition |
| Product pattern | An existing local convention for layout, component shape, styling, state, routing, or copy |
| Verification evidence | The command, browser check, screenshot, or test result that proves the work was checked |

## Layers

### Product Context

Owns user goal, workflow, existing app conventions, and product constraints.

Never:

- Replace an established product pattern without a reason.
- Build a generic showcase when the user asked for an app, tool, dashboard, or feature.
- Add explanatory in-app text that compensates for unclear interaction design.

### Implementation

Owns components, styles, state, routing, data loading, and event handling.

Never:

- Leave dead controls or placeholder behavior in product code.
- Duplicate large component logic when a local reusable pattern exists.
- Introduce a new UI library, charting library, animation engine, or icon set without clear need.
- Mutate protected office topology or bootstrap state while doing frontend work.

### Interaction

Owns behavior, focus, keyboard paths, validation, transitions, and feedback.

Never:

- Trap focus incorrectly.
- Lose user input on validation errors.
- Hide the only recovery path from an error state.
- Depend on hover-only interactions for required actions.

### Visual System

Owns layout, spacing, typography, color, density, responsive behavior, and hierarchy.

Never:

- Let text overflow or overlap.
- Use viewport-scaled font sizes.
- Create nested cards or page-section cards unless already established locally.
- Let dynamic labels, counters, loading text, or hover states resize fixed-format UI.
- Ship a one-note palette dominated by a single hue family.

### Verification

Owns automated and browser checks.

Never:

- Claim a test, screenshot, or browser check passed unless it was run.
- Treat a compiled build as proof of visual quality.
- Skip mobile inspection for user-facing UI changes.

## Contracts

### Context

- `surface.change` requires reading nearby files first.
- `new.pattern` requires a reason stronger than personal preference.
- `copy.change` must improve clarity or match existing tone.

### Interaction

- `form.submit` requires validation, error display, disabled or pending state, and recovery path.
- `modal.open` requires focus placement, keyboard close behavior, and background interaction rules.
- `async.load` requires loading, success, empty, and error treatment unless the data path cannot enter that state.
- `destructive.action` requires confirmation or undo, matching existing product behavior.

### Responsiveness

- `surface.layout` must work at desktop and mobile widths.
- `toolbar.actions` must remain reachable on mobile.
- `data.table` must provide a mobile scanning strategy: horizontal scroll, card rows, column hiding, or another explicit pattern.
- `fixed.ui` must use stable dimensions or constraints.

### Accessibility

- `interactive.element` must have an accessible name.
- `icon.button` requires an accessible label and tooltip when the meaning is not universal.
- `status.message` should be announced or positioned where the user will encounter it naturally.
- `color.meaning` must have a non-color cue when the distinction matters.

### Visual Quality

- `dashboard.surface` must prioritize scanning and comparison over decoration.
- `tool.surface` must favor density, alignment, and predictable navigation over hero composition.
- `landing.hero` may use immersive imagery, but only when the user requested a landing/brand page.
- `animation` must clarify cause, continuity, or feedback; it must not block repeated work.

### Verification

- `ui.complete` requires relevant automated checks plus browser inspection.
- `visual.claim` requires screenshot, DOM snapshot, or direct browser inspection.
- `responsive.claim` requires at least one mobile-width check.
- `interactive.claim` requires exercising the primary workflow.

## Required Handoff Evidence

The agent's final response must separate:

- Verified: checks actually run.
- Changed: files or surfaces changed.
- Risk: anything not verified, skipped, or left for follow-up.

If verification was partial, say so directly.
