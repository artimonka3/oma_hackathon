"""Package the browser-local prototype as a single offline HTML file."""
from pathlib import Path
import base64
import json
import mimetypes

root = Path(__file__).resolve().parent
assets = {}
for file in (root / 'assets').iterdir():
    if file.is_file():
        mime = mimetypes.guess_type(file.name)[0] or 'application/octet-stream'
        assets[file.name] = f'data:{mime};base64,' + base64.b64encode(file.read_bytes()).decode()

html = (root / 'index.html').read_text()
css = (root / 'styles.css').read_text()
js = (root / 'app.js').read_text()
for name, uri in assets.items():
    html = html.replace('assets/' + name, uri)
    css = css.replace('assets/' + name, uri)
    js = js.replace('assets/' + name, uri)
js = js.replace('assets/${img}.jpg', "${embeddedAssets[img + '.jpg']}")
js = js.replace("assets/${i===2?'kitchen':'living-room'}.jpg", "${embeddedAssets[(i===2?'kitchen':'living-room')+'.jpg']}")
js = js.replace("assets/${i===0?'living-room':'kitchen'}.jpg", "${embeddedAssets[(i===0?'living-room':'kitchen')+'.jpg']}")
js = js.replace("assets/${roomSelection==='living'?'living-room':'kitchen'}.jpg", "${embeddedAssets[(roomSelection==='living'?'living-room':'kitchen')+'.jpg']}")
assert 'assets/' not in js, 'Unresolved dynamic media reference'
html = html.replace('<link rel="stylesheet" href="styles.css">', '<style>' + css + '</style>')
html = html.replace('<script src="app.js" defer></script>', '')
js = 'const embeddedAssets=' + json.dumps(assets) + ';\n' + js
html = html.replace('</body>', '<script>' + js.replace('</script', '<\\/script') + '</script></body>')
out = root.parent / 'oma-prototype.html'
out.write_text(html)
print(f'Created {out.name}: {out.stat().st_size:,} bytes')
