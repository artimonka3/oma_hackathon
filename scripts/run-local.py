#!/usr/bin/env python3
"""Load local .env as data (never shell-evaluate it), then run the Go app."""
import os,shlex,subprocess
from pathlib import Path
root=Path(__file__).resolve().parents[1];os.chdir(root)
if Path('.env').exists():
 for line in Path('.env').read_text().splitlines():
  line=line.strip()
  if not line or line.startswith('#'):continue
  key,sep,value=line.partition('=')
  if not sep or not key.strip().replace('_','').isalnum():raise SystemExit('Invalid .env assignment')
  tokens=shlex.split(value,comments=True)
  os.environ.setdefault(key.strip(),' '.join(tokens))
os.environ.setdefault('APP_ADDR','127.0.0.1:8088')
os.environ.setdefault('DATABASE_URL','postgres://oma:oma_local_dev@127.0.0.1:54329/oma?sslmode=disable')
os.environ.setdefault('MEDIA_STORAGE_PATH','./var/mvp-media')
os.execvp('go',['go','run','./cmd/app'])
