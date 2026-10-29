"""Загрузчик страниц oma.by: обходит базовую cookie-проверку (hg-security)
через curl, ограничивает частоту запросов и кэширует ответы на диск.

Используется скриптами scrape_*.py. Сетевые запросы идут через curl,
потому что TLS-отпечаток python-клиентов блокируется анти-ботом oma.by.
"""
import pathlib
import re
import subprocess
import sys
import time

BASE = "https://www.oma.by"
UA = (
    "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 "
    "(KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
)
BASE_HEADERS = [
    "-H", "Accept: text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
    "-H", "Accept-Language: ru-RU,ru;q=0.9,en;q=0.8",
    "--compressed",
]


class OmaFetcher:
    def __init__(self, cache_dir, delay=0.6):
        self.cache_dir = pathlib.Path(cache_dir)
        self.cache_dir.mkdir(parents=True, exist_ok=True)
        self.jar = self.cache_dir / "cookies.txt"
        self.delay = delay
        self._last = 0.0
        self.challenges = 0

    def _throttle(self):
        wait = self.delay - (time.time() - self._last)
        if wait > 0:
            time.sleep(wait)
        self._last = time.time()

    def _curl(self, url, dest):
        return subprocess.run(
            ["curl", "-sS", *BASE_HEADERS,
             "-A", UA,
             "-c", str(self.jar), "-b", str(self.jar),
             "-o", str(dest), "-w", "%{http_code}",
             "--max-time", "45", url],
            capture_output=True, text=True, timeout=60,
        )

    def _solve_challenge(self, dest):
        body = dest.read_text(errors="replace")
        m = re.search(r"hg-security=([^;\"']+)", body)
        if not m:
            return False
        self.challenges += 1
        with self.jar.open("a") as f:
            f.write("www.oma.by\tFALSE\t/\tTRUE\t0\thg-security\t" + m.group(1) + "\n")
        time.sleep(0.3)
        return True

    def get(self, url, name=None):
        """Возвращает HTML страницы, кэшируя ответ в cache_dir/<name>.html."""
        name = name or re.sub(r"[^a-zA-Z0-9]+", "_", url.split("oma.by/")[-1])[:80]
        dest = self.cache_dir / (name + ".html")
        if dest.exists() and dest.stat().st_size > 5000:
            return dest.read_text(errors="replace")
        self._throttle()
        status = self._curl(url, dest).stdout.strip()
        body = dest.read_text(errors="replace")
        if "hg-security" in body and len(body) < 5000:
            if self._solve_challenge(dest):
                self._throttle()
                status = self._curl(url, dest).stdout.strip()
                body = dest.read_text(errors="replace")
        if status != "200":
            raise RuntimeError(f"GET {url} -> HTTP {status} ({len(body)} bytes)")
        return body


if __name__ == "__main__":
    f = OmaFetcher(sys.argv[1] if len(sys.argv) > 1 else "cache")
    html = f.get(sys.argv[2] if len(sys.argv) > 2 else BASE + "/")
    print(len(html), "bytes, challenges:", f.challenges)
