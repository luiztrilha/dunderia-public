---
name: pptx
description: Use when the task involves PowerPoint presentations or `.pptx` decks, especially when layouts, templates, placeholders, notes, charts, comments, or visual QA matter. Prefer `python-pptx` for structured edits and render slides for visual verification when layout fidelity matters.
---

# PPTX

## Workflow

1. Decide the job before touching the deck.
   - Reading text, editing an existing deck, rebuilding from a template, and creating from scratch are different tasks.
2. Inspect before editing.
   - Inventory slide count, layouts, placeholders, masters, notes, comments, charts, and media.
   - Reuse good existing slides when that is safer than rebuilding layout from scratch.
3. Preserve template fidelity.
   - Match the deck's theme, fonts, spacing, aspect ratio, and alignment system.
   - Master and layout definitions may override slide-level edits.
4. Map content to real placeholders.
   - Do not assume placeholder indexes or layout indexes are portable across decks.
   - Choose layouts based on actual content density, not generic slide instincts.
5. Run content QA and visual QA separately.
   - Text correctness is not layout correctness.
   - Check for overflow, clipping, overlap, wrong placeholder targets, leftover template text, and broken notes.
6. Re-render after meaningful edits when layout matters.
   - Prefer deck-to-PDF render and image review when possible.

## Rules

- For template-driven decks, template fidelity beats originality.
- Placeholder text, sample charts, and notes must be explicitly replaced or removed.
- Do not force dense content into layouts that do not have enough space.
- For chart-, table-, or image-heavy slides, favor layouts with real space over stacked text boxes.
- Keep important content robust across PowerPoint, LibreOffice, and conversion pipelines.
- Treat `.pptx` as OOXML when debugging fragile cases: slides, layouts, masters, notes, comments, and media are separate parts.

## Visual Verification

Preferred review loop:

1. Export deck to PDF with LibreOffice when available.
2. Render PDF pages to PNGs.
3. Inspect thumbnails or full-size pages for spacing, clipping, contrast, and consistency.

Useful commands:
```bash
soffice --headless --convert-to pdf --outdir <outdir> <input.pptx>
pdftoppm -png <outdir>/<deck>.pdf <outdir>/<deck>
```

## Dependencies

Prefer `uv` when available.

Python packages:
```bash
uv pip install python-pptx pillow
```

Fallback:
```bash
python -m pip install python-pptx pillow
```

System tools for render-based QA:
```bash
soffice --version
pdftoppm -h
```

## Output Contract

- State whether the task was text extraction, structured edit, template-driven edit, or deck creation.
- State whether visual QA was performed on rendered slides or only by structural inspection.
- Call out any unverified risk around masters, fonts, notes, charts, or cross-viewer rendering.
