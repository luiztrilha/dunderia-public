# Frontend Build Loop Playbook

This playbook turns the useful prototype workflow from `ai-native-pm-os` Module 9 into a stricter frontend-agent routine. Use it for dashboards, internal tools, feature prototypes, settings flows, CRUD surfaces, and product UI changes.

## 1. Frame The Work

Before building, answer these three questions in working notes:

- Who is this for?
- What is the one thing the interface must help them do?
- In the first 30 seconds, what should the user understand or complete?

Good framing examples:

- "A team operator needs to see which agent tasks are blocked and why."
- "A reviewer needs to compare task output before deciding whether to approve."
- "A first-time user needs to finish setup without understanding the underlying broker."

Weak framing examples:

- "Make a dashboard."
- "Improve the page."
- "Add more polish."

## 2. Choose The Build Mode

Use the lightest mode that proves the product decision:

| Mode | Use when | Deliverable |
|---|---|---|
| Logic prototype | The interaction flow is uncertain | Clickable flow with realistic state changes |
| Product implementation | The feature belongs in the app | Integrated components, state, styles, and tests |
| Data surface | The value is scanning or comparison | Dashboard/table/feed with real or representative data |
| Design refinement | The behavior exists but quality is low | Focused layout, copy, motion, accessibility, or responsive pass |

Do not overbuild. Isolated prototypes are for learning; product implementations are for durable app behavior.

## 3. Seed Prompt Pattern

When instructing an agent or sub-agent, use this shape:

```text
Build [specific surface] for [specific user].

User goal:
- [one job]

Product context:
- [where this lives]
- [nearby patterns to reuse]
- [data/state involved]

Required UI:
- [controls]
- [content]
- [navigation or layout]

Required behavior:
- [clicks]
- [validation]
- [state transitions]
- [error and empty states]

Design constraints:
- Match existing app conventions.
- Use the existing icon and styling system.
- Responsive at desktop and mobile widths.

Verification:
- Run [tests/checks].
- Open in browser and check [primary flow].
```

## 4. Preview Checklist

After the first build, inspect like a user, not like the author:

- Can the user spot the primary action without reading a paragraph?
- Does the layout support scanning, comparison, and repeated use?
- Are disabled, empty, error, loading, and success states visible where needed?
- Are controls aligned, consistently sized, and easy to hit?
- Does anything overlap, wrap badly, or jump between states?
- Does the UI work with keyboard and focus?
- Does mobile preserve the workflow instead of merely shrinking desktop?

## 5. Iteration Prompts

Use concrete feedback. Avoid "make it better."

For hierarchy:

```text
The primary action is not visually dominant enough. Make it the clear next step, reduce competing emphasis around it, and keep secondary actions available but quieter.
```

For state clarity:

```text
Add explicit empty, loading, error, and success states for this panel. Each state should preserve layout height enough to avoid major shifts.
```

For interaction logic:

```text
When the user cancels, restore the previous state. When they save invalid input, keep their entries, move focus to the first invalid field, and show an inline error.
```

For responsive behavior:

```text
At mobile width, preserve the primary workflow. Reflow the toolbar into compact icon actions, stack comparison panels vertically, and ensure buttons keep at least 44px touch height.
```

For visual polish:

```text
Tighten spacing, align edges, normalize control heights, and reduce decorative treatment. Keep the page functional and product-like, not marketing-like.
```

## 6. Verification Loop

Do not stop at code review. Run the smallest useful checks:

1. Static/type checks for touched frontend code.
2. Unit or component tests when behavior changed.
3. E2E or Playwright check for the main workflow when routing, dialogs, forms, or async state changed.
4. Desktop and mobile browser inspection for visual surfaces.
5. Console check for runtime errors.

If a check cannot run, record why and what risk remains.

## 7. Ship Decision

Ship when:

- The main workflow is complete.
- Expected states are handled.
- The UI matches existing product patterns.
- Browser verification found no blocking layout or interaction issue.
- Automated checks relevant to the touched area pass or have a documented reason for being skipped.

Do another loop when:

- The primary action is unclear.
- A required state is missing.
- Text overflows or overlaps.
- The mobile layout drops functionality.
- The browser console shows errors.
- The implementation introduces a new visual system without need.
