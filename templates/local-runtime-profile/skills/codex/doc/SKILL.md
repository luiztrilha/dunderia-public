---
name: "doc"
description: "Use when the task involves reading, creating, or editing `.docx` documents, especially when formatting, layout fidelity, styles, numbering, tracked changes, comments, fields, or section behavior matter; prefer `python-docx` plus the bundled `scripts/render_docx.py` for visual checks."
---


# DOCX Skill

## When to use
- Read or review DOCX content where layout matters (tables, diagrams, pagination).
- Create or edit DOCX files with professional formatting.
- Preserve fragile existing Word files with tracked changes, comments, fields, numbering, headers, or section-based layout.
- Validate visual layout before delivery.

## Workflow
1. Choose the job before editing.
   - Reading, style-driven generation, and fragile round-trip editing are different workflows.
   - Treat `.docx` as OOXML, not plain text. Structure matters as much as visible text.
   - For deep edits, inspect the package layout when needed: `word/document.xml`, `styles.xml`, `numbering.xml`, headers, footers, and relationship files.
   - Treat legacy `.doc` as conversion input first; treat `.docm` as macro-bearing and higher risk.
1. Prefer visual review (layout, tables, diagrams).
   - If `soffice` and `pdftoppm` are available, convert DOCX -> PDF -> PNGs.
   - Or use `scripts/render_docx.py` (requires `pdf2image` and Poppler).
   - If these tools are missing, install them or ask the user to review rendered pages locally.
2. Use `python-docx` for edits and structured creation (headings, styles, tables, lists).
   - Prefer named styles over direct formatting.
   - Extend the document's current style system instead of inventing a parallel one.
3. Preserve Word-specific systems deliberately.
   - Lists and numbering are their own system; bullets are not just pasted characters.
   - Page layout lives in sections; margins, orientation, headers, footers, and page numbering are section-level behavior.
   - Fields, bookmarks, comments, footnotes, and tracked changes need precise edits.
4. For review workflows, make minimal replacements instead of rewriting whole paragraphs.
   - Broad rewrites create noisy tracked changes and can break anchors, comments, or nearby formatting context.
5. After each meaningful change, re-render and inspect the pages.
6. If visual review is not possible, extract text with `python-docx` as a fallback and call out layout risk.
7. Keep intermediate outputs organized and clean up after final approval.

## Temp and output conventions
- Use `tmp/docs/` for intermediate files; delete when done.
- Write final artifacts under `output/doc/` when working in this repo.
- Keep filenames stable and descriptive.

## Dependencies (install if missing)
Prefer `uv` for dependency management.

Python packages:
```
uv pip install python-docx pdf2image
```
If `uv` is unavailable:
```
python3 -m pip install python-docx pdf2image
```
System tools (for rendering):
```
# macOS (Homebrew)
brew install libreoffice poppler

# Ubuntu/Debian
sudo apt-get install -y libreoffice poppler-utils
```

If installation isn't possible in this environment, tell the user which dependency is missing and how to install it locally.

## Environment
No required environment variables.

## Rendering commands
DOCX -> PDF:
```
soffice -env:UserInstallation=file:///tmp/lo_profile_$$ --headless --convert-to pdf --outdir $OUTDIR $INPUT_DOCX
```

PDF -> PNGs:
```
pdftoppm -png $OUTDIR/$BASENAME.pdf $OUTDIR/$BASENAME
```

Bundled helper:
```
python3 scripts/render_docx.py /path/to/file.docx --output_dir /tmp/docx_pages
```

## Quality expectations
- Deliver a client-ready document: consistent typography, spacing, margins, and clear hierarchy.
- Avoid formatting defects: clipped/overlapping text, broken tables, unreadable characters, or default-template styling.
- Charts, tables, and visuals must be legible in rendered pages with correct alignment.
- Do not leave unresolved revisions, orphaned comments, stale fields, broken numbering, or section drift hidden behind a visually plausible page.
- Use ASCII hyphens only. Avoid U+2011 (non-breaking hyphen) and other Unicode dashes.
- Citations and references must be human-readable; never leave tool tokens or placeholder strings.

## Final checks
- Re-render and inspect every page at 100% zoom before final delivery.
- Fix any spacing, alignment, or pagination issues and repeat the render loop.
- Confirm numbering, headers, footers, fields, tracked changes, and comments still behave correctly when they matter to the task.
- Confirm there are no leftovers (temp files, duplicate renders) unless the user asks to keep them.
