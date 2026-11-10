"""Rebuild authored JSON only in a disposable directory and compare bytes."""
from pathlib import Path
import tempfile,shutil,subprocess,json,hashlib,datetime
R=Path(__file__).resolve().parent.parent
with tempfile.TemporaryDirectory(prefix='western-author-rebuild-',dir=R) as directory:
 temp=Path(directory);shutil.copytree(R/'authoring',temp/'authoring',ignore=shutil.ignore_patterns('__pycache__'))
 result=subprocess.run(['python',str(temp/'authoring/build.py')],capture_output=True,text=True);checks=[]
 for rel in ['storypack/sui-guan-realms.json','storypack/sui-guan-realms.en-US.json','quest/sui-guan.json','quest/sui-guan.en-US.json','routes/sui-guan-routes.json']:
  source=(R/'deliverable'/rel).read_bytes();rebuilt=(temp/'deliverable'/rel).read_bytes() if (temp/'deliverable'/rel).exists() else b''
  checks.append({'file':rel,'exact':source==rebuilt,'source_sha256':hashlib.sha256(source).hexdigest(),'rebuilt_sha256':hashlib.sha256(rebuilt).hexdigest()})
 report={'utc':datetime.datetime.now(datetime.timezone.utc).isoformat(),'exit_code':result.returncode,'passed':result.returncode==0 and all(c['exact'] for c in checks),'checks':checks,'scope':'Temporary independent authoring rebuild; candidate and main source not written.','stderr':result.stderr}
 (R/'evidence/authoring_rebuild.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n');print(json.dumps(report,ensure_ascii=False,indent=2));raise SystemExit(not report['passed'])
