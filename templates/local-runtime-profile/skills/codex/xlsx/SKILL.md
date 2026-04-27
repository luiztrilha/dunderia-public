---
name: xlsx
description: Use when the task involves Excel workbooks or spreadsheet files such as `.xlsx`, `.xlsm`, `.xls`, `.csv`, or `.tsv`, especially when formulas, dates, types, formatting, workbook structure, recalculation, or template preservation matter. Prefer `pandas` for analysis and `openpyxl` when workbook fidelity matters.
---

# XLSX

## Workflow

1. Choose the tool by the job, not by habit.
   - Use `pandas` for analysis, reshaping, CSV-like imports, and tabular summaries.
   - Use `openpyxl` when formulas, styles, merged cells, comments, named ranges, or workbook preservation matter.
2. Inspect workbook structure before editing.
   - List sheets, hidden sheets, named ranges, merged cells, validations, filters, freeze panes, and print areas when the file looks template-driven.
3. Preserve live spreadsheet logic.
   - Write formulas into cells instead of hardcoding derived values when the workbook should remain editable.
   - Check absolute versus relative references before filling formulas across ranges.
4. Protect data types deliberately.
   - Long IDs, phone numbers, ZIP codes, and leading-zero values usually belong as text.
   - Remember Excel truncates numeric precision after 15 digits.
5. Handle dates as spreadsheet serials, not plain strings.
   - The 1900 date system has the false leap-day bug.
   - Some workbooks use the 1904 system.
   - Display format matters as much as the underlying value.
6. Recalculate and review before delivery.
   - `openpyxl` preserves formulas but does not calculate them.
   - Cached values may be stale after edits.
   - Do not leave formula errors such as `#REF!`, `#DIV/0!`, `#VALUE!`, `#NAME?`, or circular references behind.

## Rules

- Treat CSV as plain data exchange, not as a workbook-preserving format.
- Preserve sheet order, widths, freezes, filters, validations, print settings, and visual conventions when editing templates.
- Match existing cell styles instead of introducing a new visual system accidentally.
- For large workbooks, prefer targeted reads, explicit dtypes, and chunked or streaming workflows.
- Be careful with read-only or values-only modes; the wrong save path can flatten formulas into static values.
- Treat `.xlsm` as macro-bearing and higher risk. Do not claim macros are preserved unless you verified that exact path.

## Quality Checks

- Verify representative formulas after copying or filling ranges.
- Check dates, number formats, and text-preserved IDs in the final workbook.
- Review hidden logic carriers: named ranges, validations, conditional formatting, and hidden sheets.
- If visual presentation matters, open the workbook or export a reviewable artifact before closing the task.

## Dependencies

Prefer `uv` when available.

Python packages:
```bash
uv pip install pandas openpyxl
```

Fallback:
```bash
python -m pip install pandas openpyxl
```

## Output Contract

- State whether the task was analysis-only or workbook-preserving.
- State whether formulas were preserved, introduced, or converted to values.
- Call out recalculation or macro limitations when they were not verifiable.
