"""Смоук-проверка мок-API с каталогом oma.by: поднимает сервер, дергает
эндпоинты, проходит корзина -> котировка -> checkout -> оплата, глушит сервер.
"""
import json
import pathlib
import subprocess
import sys
import time
import urllib.request

ROOT = pathlib.Path(__file__).resolve().parent.parent.parent
ADDR = "127.0.0.1:8091"
BASE = f"http://{ADDR}/oma-api/v1"
SITE = f"http://{ADDR}"
ENV = {
    **dict(__import__("os").environ),
    "MOCKOMA_ADDR": ADDR,
    "MOCKOMA_PRODUCTS_FILE": "oma-site/data/products.json",
    "MOCKOMA_SITE_DIR": "oma-site/site",
}


def get(path):
    with urllib.request.urlopen(BASE + path, timeout=10) as r:
        return json.loads(r.read())


def post(path, body=None):
    data = json.dumps(body).encode() if body is not None else b"{}"
    req = urllib.request.Request(BASE + path, data=data, method="POST",
                                 headers={"Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=10) as r:
        return json.loads(r.read())


def main():
    server = subprocess.Popen(
        [str(ROOT / "bin" / "mockoma-test")], cwd=ROOT, env=ENV,
        stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    try:
        for _ in range(50):
            time.sleep(0.1)
            try:
                with urllib.request.urlopen(SITE + "/healthz", timeout=5) as r:
                    health = json.loads(r.read())
                break
            except Exception:
                health = None
        if not health:
            print("FAIL: сервер не поднялся")
            sys.exit(1)
        print("healthz:", health)

        cats = get("/categories")
        print("categories:", [(c["category"], c["count"]) for c in cats["categories"]])

        found = get("/products?q=" + urllib.request.quote("ламинат") + "&category=flooring")
        print("search ламинат/flooring:", len(found["products"]), "items")
        p = found["products"][0]
        print("  sample:", {k: p[k] for k in ("sku", "title", "price_minor", "unit", "pack_amount", "image_url")})

        one = get("/products/2.326214")
        print("by sku 2.326214:", one["title"], one["price_minor"], "minor")

        res = post("/carts/sync", {"project_id": "site-demo", "lines": [
            {"link_id": "l1", "sku": "2.326214", "packs": 3},
            {"link_id": "l2", "sku": "2.137843", "packs": 1},
        ]})
        print("carts/sync:", res)

        quote = post(f"/carts/{res['cart_id']}/quotes")
        print("quote:", {k: quote[k] for k in ("id", "total_minor", "currency")})

        sess = post("/checkout-sessions", {"quote_id": quote["id"]})
        order = post(f"/checkout-sessions/{sess['id']}/pay")
        print("order:", {k: order[k] for k in ("id", "status", "total_minor", "demo")})

        # витрина отдаётся тем же процессом
        with urllib.request.urlopen(SITE + "/", timeout=10) as r:
            page = r.read().decode()
        print("site index:", len(page), "bytes, содержит app.js:", "app.js" in page)
        with urllib.request.urlopen(SITE + "/style.css", timeout=10) as r:
            print("style.css:", r.status, len(r.read()), "bytes")
        print("SMOKE OK")
    finally:
        server.terminate()


if __name__ == "__main__":
    main()
