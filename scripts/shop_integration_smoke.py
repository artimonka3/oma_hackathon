#!/usr/bin/env python3
"""Full clone API acceptance test. Two real HTTP processes, temporary state, no AI spend."""
import json, os, socket, subprocess, tempfile, time, urllib.request, urllib.error
from pathlib import Path
ROOT=Path(__file__).resolve().parents[1]
def port():
 with socket.socket() as s:s.bind(('127.0.0.1',0));return s.getsockname()[1]
appport,shopport=port(),port();app=f'http://127.0.0.1:{appport}';shop=f'http://127.0.0.1:{shopport}'
processes={};logs={}
def req(base,path,body=None,token='',method=None,status=None):
 headers={'Content-Type':'application/json'}
 if token:headers['Authorization']='Bearer '+token
 r=urllib.request.Request(base+path,data=None if body is None else json.dumps(body).encode(),headers=headers,method=method or ('POST' if body is not None else 'GET'))
 try:
  with urllib.request.urlopen(r,timeout=10) as out:code=out.status;value=json.load(out)
 except urllib.error.HTTPError as e:code=e.code;value=json.load(e)
 assert code==(status or 200) or status is None and code<400,(path,code,value)
 return value
with tempfile.TemporaryDirectory(prefix='oma-shop-test-') as tmp:
 tmp=Path(tmp)
 def launch(name):
  env=dict(os.environ,DATABASE_URL='',AI_MODE='demo',DEMO_MODE='true',OMA_SERVICE_SECRET='integration-secret',OMA_IMPORT_STATE_PATH='')
  if name=='app':env.update(APP_ADDR=f'127.0.0.1:{appport}',OMA_API_BASE_URL=shop,OMA_STATE_PATH=str(tmp/'app-state.json'),MEDIA_STORAGE_PATH=str(tmp/'media'))
  else:env.update(MOCKOMA_ADDR=f'127.0.0.1:{shopport}',OMA_APP_BASE_URL=app,MOCKOMA_PRODUCTS_FILE=str(ROOT/'oma-site/data/products.json'),MOCKOMA_SITE_DIR=str(ROOT/'oma-site/site'),MOCKOMA_STATE_PATH=str(tmp/'shop-state.json'))
  logs[name]=open(tmp/(name+'.log'),'a+');processes[name]=subprocess.Popen([str(tmp/name)],cwd=ROOT,env=env,stdout=logs[name],stderr=logs[name])
  for _ in range(100):
   try:urllib.request.urlopen((app if name=='app' else shop)+'/healthz',timeout=.3).close();return
   except Exception:
    if processes[name].poll() is not None:raise AssertionError(name+' exited')
    time.sleep(.1)
  raise AssertionError(name+' readiness timeout')
 def stop(name):
  p=processes.get(name)
  if p and p.poll() is None:p.terminate();p.wait(timeout=10)
 try:
  for name,cmd in [('app','app'),('shop','mockoma')]:subprocess.run(['go','build','-o',str(tmp/name),'./cmd/'+cmd],cwd=ROOT,check=True)
  launch('shop');launch('app')
  client=req(app,'/api/v1/demo/session',{'participant':'client'})['token']
  catalog=req(app,'/api/v1/catalog',token=client)['products'];raw=req(shop,'/oma-api/v1/products')['products'];assert len(catalog)==len(raw)==611
  eligible={p['sku']:p for p in catalog if p['calculation_supported']}
  assert all(any(p['material']==kind for p in eligible.values()) for kind in ['paint','flooring','tiling'])
  assert any(p['unit']=='kg' for p in eligible.values())
  assert req(app,'/api/v1/config')['shop_connected']
  assert urllib.request.urlopen(app+'/shop/').status==200
  orders=[]
  for goal,kind,foreman in [('Покрасить стены гостиной','paint','foreman_demo_05'),('Уложить ламинат на пол','flooring','foreman_demo_06'),('Плитка в ванной','tiling','foreman_demo_04')]:
   project=req(app,'/api/v1/projects',{'goal':goal,'execution_mode':'self'},client);root='/api/v1/projects/'+project['id']
   req(app,root+'/room',{'name':'Комната','length_mm':3000,'width_mm':3000,'height_mm':2700,'dimensions_confirmed':True},client,'PUT')
   job=req(app,root+'/design-jobs',{},client)
   for _ in range(100):
    job=req(app,'/api/v1/jobs/'+job['id'],token=client)
    if job['state']=='succeeded':break
    assert job['state']!='failed',job
    time.sleep(.05)
   assert job['state']=='succeeded',job
   variant=job['result']['variants'][0];assert all(s in eligible and eligible[s]['material']==kind for s in variant['skus']),variant
   req(app,root+'/design-selections',{'variant_id':variant['id']},client)
   spec=req(app,root+'/specifications',{},client);sync=req(app,root+'/cart-sync',{},client)
   basket=req(app,root+'/application',token=client)['cart'];merchant=req(shop,'/oma-api/v1/carts/'+basket['id']);assert basket==merchant
   assert spec['total_minor']==basket['total_minor']==sum(x['packs']*x['price_minor'] for x in basket['lines'])
   analysis=req(app,root+'/analysis',token=client);assert len(analysis['foremen'])>=3,analysis
   assert any(f['foreman']['id']==foreman for f in analysis['foremen']);assert all(f['foreman']['demo'] and not f['foreman']['verified'] for f in analysis['foremen'])
   req(app,root+'/foreman',{'foreman_id':foreman},client,'PUT')
   review=req(app,root+'/reviews',{'foreman_id':foreman},client)['review']
   pro=req(app,'/api/v1/demo/session',{'participant':foreman})['token']
   req(app,'/api/v1/reviews/'+review['id']+'/decisions',{'action':'approve_current'},pro)
   checkout=req(app,root+'/checkout',{'confirmed':True},client)
   order=req(app,root+'/pay',{},client);assert order['demo'] and order['project_id']==project['id'];assert req(shop,'/oma-api/v1/orders/'+order['id'])==order
   assert req(app,root+'/pay',{},client)['id']==order['id']
   req(app,root+'/demo-delivery',{},client)
   assert req(shop,'/oma-api/v1/orders/'+order['id'])['status']=='delivered'
   orders.append((root,order['id']))
  # A raw shop payment must not bypass the project's explicit checkout authorization.
  cart=req(app,orders[0][0]+'/application',token=client)['cart']
  quote=req(shop,'/oma-api/v1/carts/'+cart['id']+'/quotes',{})
  checkout=req(shop,'/oma-api/v1/checkout-sessions',{'quote_id':quote['id']})
  req(shop,'/oma-api/v1/checkout-sessions/'+checkout['id']+'/pay',{},status=409)
  stop('app');stop('shop');launch('shop');launch('app')
  for root,oid in orders:
   state=req(app,root+'/application',token=client);assert state['order']['id']==oid and state['order']['status']=='delivered';assert req(shop,'/oma-api/v1/orders/'+oid)['status']=='delivered'
  print('SHOP INTEGRATION OK: 611 SKUs, safe unit normalization, paint/floor/tile recommendations, 8-profile catalog, basket/quote/pay callbacks, payment gating, delivery, shared storefront, app+shop restart.')
 except Exception:
  for name,l in logs.items():l.flush();l.seek(0);print(name,l.read()[-4000:])
  raise
 finally:
  for name in processes:stop(name)
  for l in logs.values():l.close()
