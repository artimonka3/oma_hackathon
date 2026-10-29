#!/usr/bin/env python3
"""Real HTTP acceptance tests, isolated data directory, including app restart."""
import os,sys,json,time,subprocess,tempfile,socket,urllib.request,urllib.error,base64
from pathlib import Path
ROOT=Path(__file__).resolve().parents[1]
with socket.socket() as sock:
 sock.bind(('127.0.0.1',0));port=sock.getsockname()[1]
BASE=f'http://127.0.0.1:{port}'
work=tempfile.TemporaryDirectory(prefix='oma-mvp-test-');proc=None
log=open(Path(work.name)/'server.log','w+')
def start():
 global proc
 env=dict(os.environ,APP_ADDR=f'127.0.0.1:{port}',OMA_API_BASE_URL='',OMA_STATE_PATH=work.name+'/state.json',MEDIA_STORAGE_PATH=work.name+'/media',DEMO_MODE='true',AI_MODE='demo',DATABASE_URL=os.getenv('MVP_TEST_DATABASE_URL',''),OMA_IMPORT_STATE_PATH='')
 proc=subprocess.Popen([str(ROOT/'bin/app')],cwd=ROOT,env=env,stdout=log,stderr=log)
 for _ in range(150):
  try:
   urllib.request.urlopen(BASE+'/healthz',timeout=.5).close();return
  except Exception:
   if proc.poll() is not None:raise RuntimeError('server exited')
   time.sleep(.1)
 raise RuntimeError('readiness timeout')
def stop():
 if proc and proc.poll() is None:proc.terminate();proc.wait(timeout=10)
def req(path,body=None,token='',method=None,status=None):
 headers={'Content-Type':'application/json'}
 if token:headers['Authorization']='Bearer '+token
 r=urllib.request.Request(BASE+path,data=None if body is None else json.dumps(body).encode(),headers=headers,method=method or ('POST' if body is not None else 'GET'))
 try:
  with urllib.request.urlopen(r,timeout=5) as out:code=out.status;val=json.load(out)
 except urllib.error.HTTPError as err:code=err.code;val=json.load(err)
 if status is not None:assert code==status,(path,code,val,status)
 elif code>=400:raise AssertionError((path,code,val))
 return val

def session(who):return req('/api/v1/demo/session',{'participant':who})['token']
def setup(goal,owner):
 p=req('/api/v1/projects',{'goal':goal,'execution_mode':'self'},owner);root='/api/v1/projects/'+p['id']
 req(root+'/room',{'name':'Гостиная','length_mm':10000,'width_mm':5000,'height_mm':2700,'dimensions_confirmed':True},owner,'PUT')
 job=req(root+'/design-jobs',{},owner)
 for _ in range(100):
  j=req('/api/v1/jobs/'+job['id'],token=owner)
  if j['state']=='succeeded':break
  if j['state']=='failed':raise AssertionError(j)
  time.sleep(.05)
 else:raise AssertionError('job timeout')
 variant=next((v for v in j['result']['variants'] if 'PAINT-MATT-WHITE' in v['skus'] or 'TILE-STONE' in v['skus']),j['result']['variants'][0])
 req(root+'/design-selections',{'variant_id':variant['id']},owner)
 sp=req(root+'/specifications',{},owner);req(root+'/cart-sync',{},owner)
 return root,sp,job

def photo(root,token):
 boundary='oma-smoke-boundary'
 raw=base64.b64decode('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jfncAAAAASUVORK5CYII=')
 body=(f'--{boundary}\r\nContent-Disposition: form-data; name="kind"\r\n\r\nphoto\r\n--{boundary}\r\nContent-Disposition: form-data; name="file"; filename="report.png"\r\nContent-Type: image/png\r\n\r\n'.encode()+raw+f'\r\n--{boundary}--\r\n'.encode())
 r=urllib.request.Request(BASE+root+'/captures',data=body,headers={'Authorization':'Bearer '+token,'Content-Type':'multipart/form-data; boundary='+boundary})
 with urllib.request.urlopen(r,timeout=5) as out:return json.load(out)['id']
try:
 start();client=session('client');foreman=session('foreman_demo_01');other=session('foreman_demo_02')
 req('/internal/v1/oma/checkout-authorizations',{},status=409)
 root,sp,job=setup('Покрасить стены гостиной',client)
 assert sp['new_purchase'][0]['sku']=='PAINT-MATT-WHITE',sp
 assert sp['new_purchase'][0]['packs']==2,sp
 req('/api/v1/jobs/'+job['id'],token=other,status=403)
 req(root+'/inventory',{'items':[{'sku':'PAINT-MATT-WHITE','amount':'10','unit':'l'}]},client,'PUT')
 sp=req(root+'/specifications',{},client);assert sp['new_purchase'][0]['packs']==1 and sp['used_inventory']==10,sp
 req(root+'/cart-sync',{},client)
 analysis=req(root+'/analysis',token=client);assert 'painting' in analysis['works']
 req(root+'/foreman',{'foreman_id':'foreman_demo_01'},client,'PUT')
 req(root+'/checkout',{'confirmed':True},client,status=422)
 review=req(root+'/reviews',{'foreman_id':'foreman_demo_01'},client)['review']
 req('/api/v1/reviews/'+review['id']+'/decisions',{'action':'approve_current'},other,status=403)
 req('/api/v1/reviews/'+review['id']+'/decisions',{'action':'approve_current'},foreman)
 quote=req(root+'/checkout',{'confirmed':True},client)
 req(root+'/cart',{'sku':'PAINT-MATT-WHITE','packs':2},client,'PATCH')
 req(root+'/pay',{},client,status=422)
 review=req(root+'/reviews',{'foreman_id':'foreman_demo_01'},client)['review']
 req('/api/v1/reviews/'+review['id']+'/decisions',{'action':'approve_current'},foreman)
 quote=req(root+'/checkout',{'confirmed':True},client);order=req(root+'/pay',{},client)
 assert req(root+'/pay',{},client)['id']==order['id']
 plan=req(root+'/default-plan',{},client);req('/api/v1/work-plans/'+plan['id']+'/confirmation',{},client)
 stage=plan['stage_ids'][0];req('/api/v1/stages/'+stage+'/start',{},foreman,status=422)
 req(root+'/demo-delivery',{},client)
 photoid=photo(root,foreman)
 urllib.request.urlopen(urllib.request.Request(BASE+'/api/v1/media/'+photoid,headers={'Authorization':'Bearer '+foreman})).close()
 foreignroot,_,_=setup('Новый пол из ламината',client)
 try:
  urllib.request.urlopen(urllib.request.Request(BASE+'/api/v1/media/'+photoid,headers={'Authorization':'Bearer '+other}));raise AssertionError('private media leaked')
 except urllib.error.HTTPError as err:assert err.code==403
 for index,stage in enumerate(plan['stage_ids']):
  req('/api/v1/stages/'+stage+'/start',{},foreman)
  rep=req('/api/v1/stages/'+stage+'/reports',{'comment':'Выполнены работы','photo_ids':[photoid]},foreman)
  body={'expected_version':rep['stage']['version'],'report_id':rep['report']['id'],'decision':'accept','acceptance_confirmed':False}
  req('/api/v1/stages/'+stage+'/decisions',body,client,status=400)
  body['acceptance_confirmed']=True
  if index==0:
   body.update(decision='request_changes',comment='Поправить угол');req('/api/v1/stages/'+stage+'/decisions',body,client)
   newer=req('/api/v1/stages/'+stage+'/reports',{'comment':'Угол исправлен','photo_ids':[photoid]},foreman)
   body.update(expected_version=newer['stage']['version'],decision='accept')
   req('/api/v1/stages/'+stage+'/decisions',body,client,status=409)
   body['report_id']=newer['report']['id']
  req('/api/v1/stages/'+stage+'/decisions',body,client)
 app=req(root+'/application',token=client);assert app['progress']==100,app
 req(root+'/finish',{},client)
 # Full coverage must empty the project-linked cart, not keep the old purchase.
 req(foreignroot+'/inventory',{'items':[{'sku':'LAMINATE-OAK','amount':'1000','unit':'pack'}]},client,'PUT',status=400)
 floor=req(foreignroot+'/application',token=client);assert floor['specification']['new_purchase'][0]['sku']=='LAMINATE-OAK'
 tile,sp,_=setup('Плитка в ванной',client);assert sp['new_purchase'][0]['sku']=='TILE-STONE'
 zero,zsp,_=setup('Покраска стен',client)
 req(zero+'/inventory',{'items':[{'sku':'PAINT-MATT-WHITE','amount':'1000','unit':'l'}]},client,'PUT')
 zsp=req(zero+'/specifications',{},client);req(zero+'/cart-sync',{},client)
 assert not zsp['new_purchase'],zsp
 assert not req(zero+'/application',token=client)['cart']['lines']
 stop();start()
 restored=req(root+'/application',token=client)
 assert restored['progress']==100 and restored['meta']['finished'] and restored['order']['id']==order['id'],restored
 assert len(restored['reports'][plan['stage_ids'][0]])==2
 assert len(req('/api/v1/projects',token=client)['projects'])>=4
 print('MVP SMOKE OK: paint/floor/tile, inventory, private jobs/media, review invalidation, idempotent purchase, delivery, photo report revision, acceptance, completion, restart.')
except Exception:
 log.flush();log.seek(0);print(log.read()[-5000:],file=sys.stderr);raise
finally:stop();log.close();work.cleanup()
