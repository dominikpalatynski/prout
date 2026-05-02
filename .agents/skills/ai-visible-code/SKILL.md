---
name: ai-visible-code
description: Generate code that is clear, structured, and easy for humans to understand and modify.
---

Write code that a human can quickly understand, debug, and extend.

## Rules

- Never put everything in one function.
- Split logic into small, named units (services, functions, classes).
- Use clear, intention-revealing names.
- Make the flow obvious (input → validation → logic → side effects → output).
- Extract side effects (DB, API, etc.) into separate functions.
- Use interfaces or types when they improve clarity.
- Prefer simple and explicit over clever.

## Output

- If needed, briefly show structure first.
- Then provide clean, organized code.

## Goal

The developer should:
- understand the code immediately,
- know where to make changes,
- feel in control (not confused by AI).