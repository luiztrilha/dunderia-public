import argparse
import json
import os
import sys
from pathlib import Path


def add_cuda_paths() -> list[str]:
    candidates = [
        Path(r"C:\Python312\Lib\site-packages\nvidia\cublas\bin"),
        Path(r"C:\Python312\Lib\site-packages\nvidia\cudnn\bin"),
    ]
    added = []
    for candidate in candidates:
        if candidate.exists():
            os.environ["PATH"] = str(candidate) + os.pathsep + os.environ.get("PATH", "")
            added.append(str(candidate))
    return added


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--input", required=True)
    parser.add_argument("--model", default="tiny")
    parser.add_argument("--language", default="pt")
    parser.add_argument("--device", default="cuda")
    parser.add_argument("--compute-type", default="float16")
    parser.add_argument("--beam-size", type=int, default=1)
    parser.add_argument("--output")
    args = parser.parse_args()

    added_paths = add_cuda_paths()

    from faster_whisper import WhisperModel

    input_path = Path(args.input).resolve()
    if not input_path.exists():
        raise FileNotFoundError(f"Arquivo nao encontrado: {input_path}")

    model = WhisperModel(args.model, device=args.device, compute_type=args.compute_type)
    segments, info = model.transcribe(
        str(input_path),
        language=args.language,
        beam_size=args.beam_size,
        vad_filter=False,
    )

    rows = []
    text_parts = []
    for segment in segments:
        row = {
            "start": round(segment.start, 2),
            "end": round(segment.end, 2),
            "text": segment.text.strip(),
        }
        rows.append(row)
        if row["text"]:
            text_parts.append(row["text"])

    result = {
        "input": str(input_path),
        "model": args.model,
        "language": info.language,
        "languageProbability": info.language_probability,
        "device": args.device,
        "computeType": args.compute_type,
        "cudaPaths": added_paths,
        "text": " ".join(text_parts).strip(),
        "segments": rows,
    }

    payload = json.dumps(result, ensure_ascii=True, indent=2)
    if args.output:
        output_path = Path(args.output).resolve()
        output_path.write_text(payload + "\n", encoding="utf-8")
    else:
        sys.stdout.write(payload + "\n")

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
