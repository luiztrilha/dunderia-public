import argparse
import json
from pathlib import Path

import fitz
from PIL import Image
from rapidocr_onnxruntime import RapidOCR


def render_pdf(input_path: Path, output_dir: Path, dpi: int) -> list[Path]:
    output_dir.mkdir(parents=True, exist_ok=True)
    doc = fitz.open(input_path)
    scale = dpi / 72.0
    matrix = fitz.Matrix(scale, scale)
    images = []
    try:
        for index, page in enumerate(doc, start=1):
            pix = page.get_pixmap(matrix=matrix, alpha=False)
            path = output_dir / f"{input_path.stem}-page-{index:03d}.png"
            pix.save(path)
            images.append(path)
    finally:
        doc.close()
    return images


def ensure_image(input_path: Path, output_dir: Path) -> list[Path]:
    output_dir.mkdir(parents=True, exist_ok=True)
    normalized_path = output_dir / f"{input_path.stem}.png"
    with Image.open(input_path) as image:
        image.convert("RGB").save(normalized_path)
    return [normalized_path]


def ocr_images(image_paths: list[Path]) -> dict:
    engine = RapidOCR()
    pages = []
    text_parts = []
    for image_path in image_paths:
        result, _ = engine(str(image_path))
        rows = []
        page_text_parts = []
        if result:
            for box, text, score in result:
                clean_text = text.strip()
                rows.append(
                    {
                        "text": clean_text,
                        "score": round(float(score), 4),
                        "box": box,
                    }
                )
                if clean_text:
                    page_text_parts.append(clean_text)
                    text_parts.append(clean_text)
        pages.append(
            {
                "image": str(image_path.resolve()),
                "text": " ".join(page_text_parts).strip(),
                "items": rows,
            }
        )

    return {
        "text": "\n\n".join(page["text"] for page in pages if page["text"]).strip(),
        "pages": pages,
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--input", required=True)
    parser.add_argument("--output", required=True)
    parser.add_argument("--images-dir", required=True)
    parser.add_argument("--dpi", type=int, default=200)
    args = parser.parse_args()

    input_path = Path(args.input).resolve()
    output_path = Path(args.output).resolve()
    images_dir = Path(args.images_dir).resolve()

    if not input_path.exists():
        raise FileNotFoundError(f"Arquivo nao encontrado: {input_path}")

    if input_path.suffix.lower() == ".pdf":
        image_paths = render_pdf(input_path, images_dir, args.dpi)
        source_type = "pdf"
    else:
        image_paths = ensure_image(input_path, images_dir)
        source_type = "image"

    ocr_result = ocr_images(image_paths)
    payload = {
        "input": str(input_path),
        "sourceType": source_type,
        "dpi": args.dpi,
        "images": [str(path.resolve()) for path in image_paths],
        "text": ocr_result["text"],
        "pages": ocr_result["pages"],
    }
    output_path.parent.mkdir(parents=True, exist_ok=True)
    output_path.write_text(json.dumps(payload, ensure_ascii=True, indent=2) + "\n", encoding="utf-8")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
