"""Скрапер каталога oma.by для мок-магазина проекта ОМА.

Фазы:
  1. Обход разделов каталога (BFS до глубины 2) с пагинацией; карточки товаров
     разбираются из data-атрибутов листинга (название, sku, цена, бренд,
     категория, наличие, картинка, URL).
  2. Загрузка страниц товаров: JSON-LD (описание, бренд, цена, наличие) и
     «Единицы измерения» из таблицы характеристик.
  3. Сборка итоговых файлов: data/products.json (контракт oma.Product +
     расширенные поля), data/categories.json.

Результат — демонстрационные данные для мок-API; реальный магазин ОМА не
задействован в проекте. Запросы ограничены по частоте и кэшируются.
"""
import json
import pathlib
import re
import sys
import time

sys.path.insert(0, str(pathlib.Path(__file__).parent))
from omafetch import OmaFetcher, BASE

ROOT = pathlib.Path(__file__).resolve().parent.parent
CACHE = ROOT / "raw" / "cache"
DATA = ROOT / "data"

# Разделы, релевантные сценарию ремонта (ОМА.Собери / ОМА.Прораб).
# instrument и krepezh — первые: их листинги — навигаторы без карточек,
# товары находятся в подкатегориях, которые должны успеть до лимита.
SEEDS = [
    "/instrument-c",                      # инструмент
    "/krepezh-c",                          # крепёж
    "/kraski-c",                          # краски
    "/gruntovki-c",                       # грунтовки
    "/emali-c",                           # эмали
    "/laki-c",                             # лаки
    "/oboi-c",                            # обои
    "/napolnye-pokrytiya-c",              # напольные покрытия
    "/keramicheskaya-plitka-i-zatirka-c", # плитка и затирка
    "/dveri-c",                           # двери
]

MAX_DEPTH = 2          # BFS: seed -> подкатегории -> под-подкатегории
MAX_PAGES_PER_SECTION = 3
MAX_PRODUCTS = 700     # верхняя граница уникальных карточек
PRODUCT_DELAY = 0.55

CARD_RE = re.compile(
    r'<div class="product-item[^"]*js_cat_list_item[^"]*".*?'
    r'(?=<div class="product-item[^"]*js_cat_list_item|\Z)', re.S)


def attr(block, name):
    m = re.search(rf'{name}="([^"]*)"', block)
    return m.group(1) if m else ""


def parse_listing(html, section):
    cards = {}
    for block in CARD_RE.findall(html):
        href = attr(block, "href")
        m = re.search(r'href="(/[a-z0-9-]+-\d{5,}-p)"', block)
        sku = attr(block, "data-sku")
        if not m or not sku:
            continue
        url = m.group(1)
        price = attr(block, "data-ga-product-price")
        if not price:
            continue
        cards[url] = {
            "sku": sku,
            "url": url,
            "title": attr(block, "data-ga-product-name"),
            "brand": attr(block, "data-ga-product-brand"),
            "price": float(price),
            "old_price": float(attr(block, "data-ga-product-price-old") or 0),
            "available": attr(block, "data-available") == "true",
            "category_path": attr(block, "data-ga-category-path"),
            "category_leaf": attr(block, "data-ga-category-last"),
            "country": attr(block, "data-dy-manufacture"),
            "section": section,
            "image": "",
        }
        im = re.search(r'src="(/upload/[^"]+)"', block)
        if im:
            cards[url]["image"] = BASE + im.group(1)
    return cards


def subcategory_links(html, self_path):
    out = []
    for h in sorted(set(re.findall(r'href="(/[a-z0-9-]+-c)"', html))):
        if h != self_path and h not in SEEDS:
            out.append(h)
    return out


def crawl():
    f = OmaFetcher(CACHE)
    seen_sections, queue, products = set(), [(s, 0) for s in SEEDS], {}
    while queue and len(products) < MAX_PRODUCTS:
        queue.sort(key=lambda x: x[1])  # сначала сиды, потом их подкатегории
        path, depth = queue.pop(0)
        if path in seen_sections or depth > MAX_DEPTH:
            continue
        seen_sections.add(path)
        try:
            html = f.get(BASE + path)
        except RuntimeError as e:
            print(f"[crawl] пропущен {path}: {e}")
            continue
        page_cards = parse_listing(html, path)
        products.update(page_cards)
        print(f"[crawl] {path} depth={depth}: +{len(page_cards)} (total {len(products)})")
        for sub in subcategory_links(html, path):
            if sub not in seen_sections:
                queue.append((sub, depth + 1))
        # пагинация
        for page in range(2, MAX_PAGES_PER_SECTION + 1):
            if len(products) >= MAX_PRODUCTS or f"PAGEN_1={page}" not in html:
                break
            page_html = f.get(BASE + f"{path}?PAGEN_1={page}")
            extra = parse_listing(page_html, path)
            new = {k: v for k, v in extra.items() if k not in products}
            if not new:
                break
            products.update(new)
            print(f"[crawl] {path} page {page}: +{len(new)} (total {len(products)})")
            html = page_html
        time.sleep(0.2)
    return f, products


UNITS_RE = re.compile(r"Единицы измерения\s*</[^>]+>\s*(?:<[^>]+>\s*)*([А-Яа-яЁёA-Za-z0-9/ .]+)")


def parse_product_page(html):
    out = {"description": "", "units": "", "image": ""}
    m = re.search(r'<script type="application/ld\+json">\s*(\{.*?"@type"\s*:\s*"Product".*?)\s*</script>', html, re.S)
    if not m:
        m = re.search(r'<script type="application/ld\+json">(\s*\{"@context".*?)</script>', html, re.S)
    if m:
        try:
            d = json.loads(m.group(1))
            if d.get("@type") == "Product":
                out["description"] = (d.get("description") or "").strip()
                out["sku"] = d.get("sku", "")
        except json.JSONDecodeError:
            pass
    u = UNITS_RE.search(html)
    if u:
        out["units"] = u.group(1).strip()
    return out


def enrich(f, products):
    keys = list(products)
    for i, url in enumerate(keys):
        if len(sys.argv) > 1 and int(sys.argv[1]) and i >= int(sys.argv[1]):
            print("лимит товара из argv, стоп")
            break
        html = f.get(BASE + url)
        info = parse_product_page(html)
        for k, v in info.items():
            if v:  # не затираем данные листинга пустыми значениями
                products[url][k] = v
        if (i + 1) % 25 == 0:
            print(f"[enrich] {i + 1}/{len(keys)}")
    return products


def main():
    f, products = crawl()
    print(f"итого карточек: {len(products)}, challenges: {f.challenges}")
    products = enrich(f, products)
    CACHE.parent.mkdir(exist_ok=True)
    (CACHE.parent / "scraped-products.json").write_text(
        json.dumps(products, ensure_ascii=False, indent=1))
    print(f"сохранено: {len(products)} товаров -> oma-site/raw/scraped-products.json")


if __name__ == "__main__":
    main()
