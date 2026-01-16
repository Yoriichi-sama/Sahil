import contextlib
import io
import json
import os
import re
import warnings
from collections import defaultdict

import cv2
import numpy as np

os.environ["DISABLE_MODEL_SOURCE_CHECK"] = "True"
os.environ["GLOG_minloglevel"] = "3"
os.environ["FLAGS_minloglevel"] = "3"
os.environ["KMP_WARNINGS"] = "0"
warnings.filterwarnings("ignore")

from paddleocr import TextRecognition

FOLDER = r"C:\Users\jatin\komi\checkmodel\output"
OUT_DIR = r"C:\Users\jatin\komi\checkmodel\output_readability"

SCORE_THRESHOLD = 0.70
ROTATIONS = [0, 90, -90, 180]

UPSCALE_MIN_SIDE = 180
UPSCALE_FACTOR = 4

ANGLE_LIMIT_ABS = 100.0
TEMP_MAX = 80.0

EXTS = (".jpg", ".jpeg", ".png", ".bmp", ".webp")


def ensure_dir(p):
    os.makedirs(p, exist_ok=True)


def imread_any_gray(path):
    data = np.fromfile(path, dtype=np.uint8)
    if data.size == 0:
        return None
    return cv2.imdecode(data, cv2.IMREAD_GRAYSCALE)


def rotate_gray(gray, deg):
    if deg == 90:
        return cv2.rotate(gray, cv2.ROTATE_90_CLOCKWISE)
    if deg == -90:
        return cv2.rotate(gray, cv2.ROTATE_90_COUNTERCLOCKWISE)
    if deg == 180:
        return cv2.rotate(gray, cv2.ROTATE_180)
    return gray


def upscale_if_needed(gray):
    h, w = gray.shape[:2]
    if min(h, w) < UPSCALE_MIN_SIDE:
        gray = cv2.resize(
            gray,
            (w * UPSCALE_FACTOR, h * UPSCALE_FACTOR),
            interpolation=cv2.INTER_CUBIC,
        )
    return gray


def clahe(gray, clip=3.0):
    return cv2.createCLAHE(clipLimit=float(clip), tileGridSize=(8, 8)).apply(gray)


def denoise(gray):
    return cv2.fastNlMeansDenoising(gray, None, 10, 7, 21)


def unsharp(gray, amount=1.05, radius=1.05):
    blur = cv2.GaussianBlur(gray, (0, 0), radius)
    out = cv2.addWeighted(gray, 1.0 + amount, blur, -amount, 0)
    return np.clip(out, 0, 255).astype(np.uint8)


def normalize_contrast(gray):
    return cv2.normalize(gray, None, 0, 255, cv2.NORM_MINMAX).astype(np.uint8)


def enhance_for_ocr(gray):
    g = upscale_if_needed(gray)
    g = denoise(g)
    g = clahe(g, 3.0)
    g = unsharp(g, 1.05, 1.05)
    g = normalize_contrast(g)
    return g


def ocr_once(rec_model, gray):
    rgb = cv2.cvtColor(gray, cv2.COLOR_GRAY2RGB)
    res = rec_model.predict(input=[rgb], batch_size=1)[0]
    try:
        j = res.json
        txt = (j["res"]["rec_text"] or "").strip()
        score = float(j["res"]["rec_score"])
    except Exception:
        txt, score = "", 0.0
    return txt, score


def extract_num_str(txt: str):
    if not txt:
        return None
    t = txt.replace(" ", "")
    m = re.search(r"[+-]?\d+(\.\d+)?", t)
    return m.group(0) if m else None


def sign_first(txt: str):
    t = (txt or "").replace(" ", "")
    return bool(re.match(r"^[+-]\d", t))


def has_temp_hint(txt: str):
    t = (txt or "").lower()
    return ("c" in t) or ("°" in t)


def has_depth_hint(txt: str):
    t = (txt or "").lower()
    return "m" in t


def normalize_angle(num_str: str):
    if not num_str:
        return None
    s = num_str.replace(" ", "")
    if not s.startswith(("+", "-")):
        s = "+" + s
    try:
        v = float(s)
    except Exception:
        return None
    if abs(v) > ANGLE_LIMIT_ABS:
        return None
    return s


def depth_valid(num_str: str):
    if not num_str:
        return False
    s = num_str.strip()
    if s.startswith(("+", "-")):
        s = s[1:]
    s = s.replace(" ", "")
    if not re.match(r"^\d+(\.\d+)?$", s):
        return False
    left = s.split(".")[0]
    return len(left) <= 3


def repair_temp(num_str: str):
    if not num_str:
        return None
    s = num_str.strip()
    if s.startswith(("+", "-")):
        s = s[1:]
    s = s.replace(" ", "")
    s = re.sub(r"[^0-9.]", "", s)
    if not s:
        return None
    if "." in s:
        try:
            v = float(s)
        except Exception:
            return None
        return v if v <= TEMP_MAX else None
    if not s.isdigit():
        return None
    if len(s) <= 2:
        v = float(int(s))
        return v if v <= TEMP_MAX else None
    candidates = []
    if len(s) >= 3:
        candidates.append(float(int(s[:2])))
        candidates.append(float(int(s[-2:])))
    if len(s) == 3:
        candidates.append(float(s[:2] + "." + s[2]))
    valid = [c for c in candidates if c <= TEMP_MAX]
    if not valid:
        return None
    decimals = [v for v in valid if isinstance(v, float) and abs(v - int(v)) > 1e-9]
    if decimals:
        return decimals[0]
    return min(valid)


def score_key(txt, score):
    t = (txt or "").replace(" ", "")
    bonus = 0.0
    if extract_num_str(t):
        bonus += 0.05
    if "." in t:
        bonus += 0.05
    if sign_first(t):
        bonus += 0.08
    return float(score) + bonus


def best_ocr_text(rec_model, gray_img):
    best = {"txt": "", "sc": 0.0, "rot": 0}
    for rot in ROTATIONS:
        g = rotate_gray(gray_img, rot)
        g = enhance_for_ocr(g)
        txt, sc = ocr_once(rec_model, g)
        if score_key(txt, sc) > score_key(best["txt"], best["sc"]):
            best = {"txt": txt, "sc": sc, "rot": rot}
    return best


def choose_rotation(rec_model, angle_imgs, temp_imgs, depth_imgs):
    probe = {}
    for rot in ROTATIONS:
        best = {"txt": "", "sc": 0.0}
        for src, img in angle_imgs.items():
            g = rotate_gray(img, rot)
            g = enhance_for_ocr(g)
            txt, sc = ocr_once(rec_model, g)
            if score_key(txt, sc) > score_key(best["txt"], best["sc"]):
                best = {"txt": txt, "sc": sc}
        probe[rot] = best

    sign_rots = [r for r in ROTATIONS if sign_first(probe[r]["txt"])]
    if sign_rots:
        return max(sign_rots, key=lambda r: probe[r]["sc"]), "angle_sign_first"

    hint_scores = {}
    for rot in ROTATIONS:
        sc_sum = 0.0
        for src, img in temp_imgs.items():
            g = rotate_gray(img, rot)
            g = enhance_for_ocr(g)
            txt, sc = ocr_once(rec_model, g)
            if has_temp_hint(txt):
                sc_sum += sc + 0.15
        for src, img in depth_imgs.items():
            g = rotate_gray(img, rot)
            g = enhance_for_ocr(g)
            txt, sc = ocr_once(rec_model, g)
            if has_depth_hint(txt):
                sc_sum += sc + 0.15
        hint_scores[rot] = sc_sum

    best_rot = max(hint_scores, key=lambda r: hint_scores[r])
    if hint_scores[best_rot] > 0.0:
        return best_rot, "unit_hint"

    return max(ROTATIONS, key=lambda r: probe[r]["sc"]), "fallback_best_angle_score"


def read_field_best(rec_model, imgs, rot):
    best = {"src": "none", "txt": "", "sc": 0.0}
    for src in ["sr", "v7", "v8"]:
        img = imgs[src]
        g = rotate_gray(img, rot)
        g = enhance_for_ocr(g)
        txt, sc = ocr_once(rec_model, g)
        if score_key(txt, sc) > score_key(best["txt"], best["sc"]):
            best = {"src": src, "txt": txt, "sc": sc}
    return best


def detect_field_and_type(fn: str):
    s = fn.lower()
    field = None
    if s.startswith("angle"):
        field = "angle"
    elif s.startswith("temp"):
        field = "temp"
    elif s.startswith("depth"):
        field = "depth"
    else:
        return None, None
    if "_sr" in s:
        typ = "sr"
    elif "_v7" in s:
        typ = "v7"
    elif "_v8" in s:
        typ = "v8"
    else:
        return None, None
    return field, typ


def scan_groups(folder):
    groups = defaultdict(dict)
    for root, _, files in os.walk(folder):
        gid = os.path.basename(root)
        for fn in files:
            if not fn.lower().endswith(EXTS):
                continue
            field, typ = detect_field_and_type(fn)
            if not field:
                continue
            groups[gid][f"{field}_{typ}"] = os.path.join(root, fn)
    return groups


def main():
    ensure_dir(OUT_DIR)

    with (
        contextlib.redirect_stdout(io.StringIO()),
        contextlib.redirect_stderr(io.StringIO()),
    ):
        rec_model = TextRecognition(model_name="en_PP-OCRv5_mobile_rec", device="cpu")

    groups = scan_groups(FOLDER)
    if not groups:
        print("No crops found in:", FOLDER)
        return

    required = [
        "angle_sr", "angle_v7", "angle_v8",
        "temp_sr", "temp_v7", "temp_v8",
        "depth_sr", "depth_v7", "depth_v8",
    ]

    out_jsonl = os.path.join(OUT_DIR, "ocr_results.jsonl")
    ok = 0
    bad = 0

    with open(out_jsonl, "w", encoding="utf-8") as f:
        for gid, d in sorted(groups.items(), key=lambda x: x[0].lower()):
            missing = [k for k in required if k not in d]
            if missing:
                bad += 1
                rec = {"id": gid, "status": "MISTAKE", "reason": "missing_files", "missing": missing}
                f.write(json.dumps(rec, ensure_ascii=False) + "\n")
                print(gid, "| MISTAKE | missing")
                continue

            imgs = {}
            for k in required:
                g = imread_any_gray(d[k])
                if g is None:
                    imgs = None
                    break
                imgs[k] = g

            if imgs is None:
                bad += 1
                rec = {"id": gid, "status": "MISTAKE", "reason": "cant_read_image"}
                f.write(json.dumps(rec, ensure_ascii=False) + "\n")
                print(gid, "| MISTAKE | cant_read_image")
                continue

            angle_imgs = {"sr": imgs["angle_sr"], "v7": imgs["angle_v7"], "v8": imgs["angle_v8"]}
            temp_imgs = {"sr": imgs["temp_sr"], "v7": imgs["temp_v7"], "v8": imgs["temp_v8"]}
            depth_imgs = {"sr": imgs["depth_sr"], "v7": imgs["depth_v7"], "v8": imgs["depth_v8"]}

            rot, rot_reason = choose_rotation(rec_model, angle_imgs, temp_imgs, depth_imgs)

            angle_best = read_field_best(rec_model, angle_imgs, rot)
            temp_best = read_field_best(rec_model, temp_imgs, rot)
            depth_best = read_field_best(rec_model, depth_imgs, rot)

            angle_num = extract_num_str(angle_best["txt"])
            temp_num = extract_num_str(temp_best["txt"])
            depth_num = extract_num_str(depth_best["txt"])

            angle_final = normalize_angle(angle_num)
            temp_final = repair_temp(temp_num)
            depth_final = depth_num if depth_valid(depth_num) else None

            mistakes = []
            if angle_final is None:
                mistakes.append("angle_invalid")
            if temp_final is None:
                mistakes.append("temp_invalid")
            if depth_final is None:
                mistakes.append("depth_invalid")
            if angle_best["sc"] < SCORE_THRESHOLD:
                mistakes.append("angle_low_score")
            if temp_best["sc"] < SCORE_THRESHOLD:
                mistakes.append("temp_low_score")
            if depth_best["sc"] < SCORE_THRESHOLD:
                mistakes.append("depth_low_score")

            status = "OK" if not mistakes else "MISTAKE"
            if status == "OK":
                ok += 1
                print(
                    f"{gid} | OK | rot={rot}({rot_reason}) | temp={temp_final} | depth={depth_final} | angle={angle_final}"
                )
            else:
                bad += 1
                print(
                    f"{gid} | MISTAKE | rot={rot}({rot_reason}) | temp={temp_final} | depth={depth_final} | angle={angle_final}"
                )

            rec = {
                "id": gid,
                "status": status,
                "rotation": rot,
                "rotation_reason": rot_reason,
                "values": {
                    "temp": temp_final if temp_final is not None else "UNREADABLE",
                    "depth": depth_final if depth_final is not None else "UNREADABLE",
                    "angle": angle_final if angle_final is not None else "UNREADABLE",
                },
                "sources": {
                    "temp": temp_best["src"],
                    "depth": depth_best["src"],
                    "angle": angle_best["src"],
                },
                "scores": {
                    "temp": float(temp_best["sc"]),
                    "depth": float(depth_best["sc"]),
                    "angle": float(angle_best["sc"]),
                },
                "raw_text": {
                    "temp": temp_best["txt"],
                    "depth": depth_best["txt"],
                    "angle": angle_best["txt"],
                },
                "mistakes": mistakes,
            }
            f.write(json.dumps(rec, ensure_ascii=False) + "\n")

    print("\nSUMMARY")
    print("OK:", ok)
    print("MISTAKE:", bad)
    print("JSONL:", out_jsonl)


if __name__ == "__main__":
    main()
