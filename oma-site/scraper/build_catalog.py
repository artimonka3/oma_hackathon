"""Сборка итоговых данных мок-магазина из сырого скрапа oma.by.

Читает raw/scraped-products.json и пишет:
  data/products.json   — контракт oma.Product + расширенные поля витрины
  data/categories.json — категории с русскими названиями и количеством

Маппинг категорий — на словарь проекта (paint/primer/wallpaper/flooring/
tiling/door/accessory/tool/misc), единицы — на pcs/m2/l/kg/roll/set/pack.
"""
import json
import pathlib
import re

ROOT = pathlib.Path(__file__).resolve().parent.parent
RAW = ROOT / "raw" / "scraped-products.json"
DATA = ROOT / "data"

CATEGORY_LABELS = {
    "paint": "Краски и эмали",
    "primer": "Грунтовки",
    "coating": "Лаки, масла, пропитки",
    "wallpaper": "Обои",
    "flooring": "Напольные покрытия",
    "tiling": "Плитка и затирка",
    "door": "Двери",
    "accessory": "Плинтусы, порожки, фурнитура",
    "tool": "Инструмент",
    "misc": "Прочее",
}

# Лист категории oma.by -> (категория контракта, материал для расчёта soberi)
LEAF_MAP = {
    "Краски": ("paint", "paint"),
    "Эмали": ("paint", "paint"),
    "Грунтовки": ("primer", "primer"),
    "Лаки": ("coating", ""),
    "Пропитки": ("coating", ""),
    "Масла": ("coating", ""),
    "ЛКМ аэрозольные": ("coating", ""),
    "Растворители и очистители": ("misc", ""),
    "Обои": ("wallpaper", ""),
    "Стеклохолст": ("wallpaper", ""),
    "Ламинат": ("flooring", "flooring"),
    "Линолеум": ("flooring", "flooring"),
    "Ковры": ("flooring", "flooring"),
    "Ковровые покрытия": ("flooring", "flooring"),
    "Искусственная трава": ("flooring", ""),
    "Подложка": ("accessory", ""),
    "Напольные плинтуса": ("accessory", ""),
    "Порожки и обводы": ("accessory", ""),
    "Наличники, коробки, доборные элементы": ("accessory", ""),
    "Уголки, профили и ленты": ("accessory", ""),
    "Керамическая плитка": ("tiling", "tiling"),
    "Мозаика": ("tiling", "tiling"),
    "Фуга": ("tiling", ""),
    "Системы выравнивания плитки": ("tiling", ""),
    "Двери межкомнатные": ("door", ""),
    "Двери входные": ("door", ""),
    "Ручки дверные": ("accessory", ""),
    "Защелки для межкомнатных дверей": ("accessory", ""),
}

UNIT_MAP = {
    "шт": "pcs", "кв. м": "m2", "рулон": "roll",
    "упаковка": "pack", "трубка": "pcs", "бут": "pcs",
}

VOLUME_RE = re.compile(r"(\d+(?:[.,]\d+)?)\s*(л|литр|кг|kg|м2|м²|мл)\b")
AREA_S_RE = re.compile(r"S\s*=\s*(\d+(?:[.,]\d+)?)", re.I)


def detect_pack(title):
    """Объём/вес/площадь из названия: '10л' -> (10.0, 'л')."""
    out = []
    for m in VOLUME_RE.finditer(title.lower()):
        val = float(m.group(1).replace(",", "."))
        unit = m.group(2)
        if unit == "мл":
            val, unit = val / 1000, "л"
        out.append((val, unit))
    return out[-1] if out else (1.0, "")


def normalize(s):
    return s.replace("\xa0", " ").strip()


def map_product(item):
    leaf = normalize(item.get("category_leaf", ""))
    category, material = LEAF_MAP.get(leaf, ("misc", ""))
    units = item.get("units", "шт")
    title = normalize(item.get("title", ""))

    unit, pack = UNIT_MAP.get(units, ""), 1.0
    if units in ("ведро", "банка", "канистра", "бут"):
        vol, vol_unit = detect_pack(title)
        if vol_unit in ("л", "кг"):
            unit, pack = vol_unit, vol
        else:
            unit = "pcs"
    elif units == "кв. м":
        unit = "m2"
        if leaf in ("Ламинат",):
            vol, vol_unit = detect_pack(title)
            if vol_unit == "м2" and vol > 1:
                pack = vol
    elif units == "рулон":
        unit = "roll"
    elif not unit:
        unit = "pcs"

    # Ламинат в упаковках: площадь берём из «S=2.22» в названии, продаётся за м2.
    if leaf == "Ламинат":
        m = AREA_S_RE.search(title)
        if m:
            area = float(m.group(1).replace(",", "."))
            if 0.1 <= area <= 10:
                unit, pack = "m2", area

    out = {
        "sku": item["sku"],
        "title": title,
        "category": category,
        "unit": unit,
        "pack_amount": pack,
        "price_minor": int(round(item["price"] * 100)),
        "currency": "BYN",
        "stock": 50 if item.get("available") else 0,
        "demo": True,
        "source_url": "https://www.oma.by" + item.get("url", ""),
        "image_url": item.get("image", ""),
        "brand": item.get("brand", ""),
        "oma_category": leaf,
        "units_raw": units,
    }
    if item.get("old_price"):
        out["old_price_minor"] = int(round(item["old_price"] * 100))
    if material:
        out["material"] = material
        if material == "paint":
            out["coverage"] = 0.1
    desc = normalize(item.get("description") or "")
    if desc:
        out["description"] = desc[:600]
    if item.get("country"):
        out["country"] = item["country"]
    return out


def main():
    raw = json.loads(RAW.read_text())
    seen, products = set(), []
    for item in raw.values():
        p = map_product(item)
        if p["sku"] in seen or not p["title"] or not p["price_minor"]:
            continue
        seen.add(p["sku"])
        products.append(p)
    products.sort(key=lambda p: (p["category"], p["title"]))

    DATA.mkdir(parents=True, exist_ok=True)
    (DATA / "products.json").write_text(json.dumps(products, ensure_ascii=False, indent=1))

    counts = {}
    for p in products:
        counts[p["category"]] = counts.get(p["category"], 0) + 1
    categories = [
        {"category": c, "title": CATEGORY_LABELS.get(c, c), "count": n}
        for c, n in sorted(counts.items(), key=lambda kv: -kv[1])
    ]
    (DATA / "categories.json").write_text(json.dumps(categories, ensure_ascii=False, indent=1))

    print(f"товаров: {len(products)}; категории: {counts}")
    bad = [p for p in products if p["unit"] == "pcs" and p["pack_amount"] != 1]
    print(f"подозрительные pcs c pack != 1: {len(bad)}")
    for p in products[:3]:
        print(json.dumps(p, ensure_ascii=False)[:220])


if __name__ == "__main__":
    main()
