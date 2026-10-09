#!/usr/bin/env python3
"""Synthetic CNI smoke test. Never writes an activation report or uses vendor credentials.

Requires an isolated cluster and a suspended Fleet whose proxy selects app=proxy in
namespace proxy on port 3128. Fixture resources remain for inspection until --cleanup.
The proxy is a test fixture, not a production proxy implementation.
"""
import argparse
import json
import subprocess
import time

PROXY = r'''
import http.server, ipaddress, select, socket, threading
class Proxy(http.server.BaseHTTPRequestHandler):
    def log_message(self, *args): pass
    def do_CONNECT(self):
        if self.path != 'api.anthropic.com:443':
            self.send_error(403); return
        addresses = socket.getaddrinfo('api.anthropic.com', 443, type=socket.SOCK_STREAM)
        if not addresses or any(not ipaddress.ip_address(a[4][0]).is_global for a in addresses):
            self.send_error(403); return
        remote = socket.socket(addresses[0][0], socket.SOCK_STREAM)
        try:
            remote.settimeout(5); remote.connect(addresses[0][4])
            self.send_response(200); self.end_headers()
            remote.settimeout(None)
            while True:
                ready, _, _ = select.select([self.connection, remote], [], [], 10)
                if not ready: break
                for src in ready:
                    data = src.recv(65536)
                    if not data: return
                    (remote if src is self.connection else self.connection).sendall(data)
        finally: remote.close()
    def do_GET(self): self.send_error(403)
http.server.ThreadingHTTPServer(('0.0.0.0',3128), Proxy).serve_forever()
'''

def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--kubeconfig', required=True)
    p.add_argument('--context', required=True)
    p.add_argument('--namespace', default='cloud-operator-system')
    p.add_argument('--fleet', required=True)
    p.add_argument('--image', required=True)
    p.add_argument('--cleanup', action='store_true')
    a = p.parse_args()
    k = ['kubectl', '--kubeconfig', a.kubeconfig, '--context', a.context]
    def run(*args, data=None):
        return subprocess.check_output(k + list(args), input=data, text=True, timeout=150)
    def apply(doc): run('apply', '-f', '-', data=json.dumps(doc))
    if a.cleanup:
        run('delete', 'pods,configmaps,services', '-A', '-l', 'operator-test=network-smoke', '--ignore-not-found')
        return
    f = json.loads(run('get', 'clauderunnerfleet', a.fleet, '-n', a.namespace, '-o', 'json'))
    if not f['spec'].get('suspended', True): raise SystemExit('Fleet must be suspended')
    proxy = f['spec']['execution']['proxy']
    if proxy != {'url':'http://proxy.proxy.svc:3128','namespace':'proxy','podLabels':{'app':'proxy'},'port':3128}:
        raise SystemExit('Fleet does not select the synthetic proxy fixture')
    policies = json.loads(run('get','networkpolicy','-n',a.namespace,'-o','json'))['items']
    owned = [policy for policy in policies if any(owner.get('uid') == f['metadata']['uid'] for owner in policy['metadata'].get('ownerReferences', []))]
    # Lifecycle changes are frozen in receipt digests but do not change network selectors.
    selected = [policy for policy in owned if policy['spec']['podSelector'].get('matchLabels', {}).get('runners.biwsantang.github.io/role') == 'session'
                and policy['metadata'].get('annotations', {}).get('runners.biwsantang.github.io/declared-policy-digest') == f['status']['policyDigest']]
    if len(selected) != 1: raise SystemExit('Expected one current Fleet session network policy')
    network_labels = selected[0]['spec']['podSelector']['matchLabels']
    revision = f['status']['policyDigest']
    apply({'apiVersion':'v1','kind':'Namespace','metadata':{'name':'proxy'}})
    labels = {'operator-test':'network-smoke'}
    apply({'apiVersion':'v1','kind':'ConfigMap','metadata':{'name':'network-proxy','namespace':'proxy','labels':labels},'data':{'proxy.py':PROXY}})
    security = {'runAsNonRoot':True,'runAsUser':1000,'runAsGroup':1000,'allowPrivilegeEscalation':False,'readOnlyRootFilesystem':True,'capabilities':{'drop':['ALL']},'seccompProfile':{'type':'RuntimeDefault'}}
    def pod(name, ns, podlabels, command, fixture=False):
        volumes = [{'name':'tmp','emptyDir':{'sizeLimit':'64Mi'}}]
        mounts = [{'name':'tmp','mountPath':'/tmp'}]
        if fixture:
            volumes.append({'name':'script','configMap':{'name':'network-proxy'}})
            mounts.append({'name':'script','mountPath':'/fixture','readOnly':True})
        return {'apiVersion':'v1','kind':'Pod','metadata':{'name':name,'namespace':ns,'labels':dict(labels,**podlabels)},'spec':{'automountServiceAccountToken':False,'enableServiceLinks':False,'restartPolicy':'Never','securityContext':{'fsGroup':1000},'volumes':volumes,'containers':[{'name':'test','image':a.image,'imagePullPolicy':'IfNotPresent','securityContext':security,'resources':{'requests':{'cpu':'100m','memory':'64Mi'},'limits':{'memory':'256Mi'}},'volumeMounts':mounts,'command':command}]}}
    apply(pod('network-proxy','proxy',{'app':'proxy'},['python3','/fixture/proxy.py'],True))
    apply(pod('network-private','proxy',{'app':'private'},['python3','-m','http.server','8080','--directory','/tmp']))
    for name, port in [('proxy',3128),('private',8080)]:
        apply({'apiVersion':'v1','kind':'Service','metadata':{'name':name,'namespace':'proxy','labels':labels},'spec':{'selector':{'app':name},'ports':[{'port':port,'targetPort':port}]}})
    run('wait','--for=condition=Ready','pod/network-proxy','pod/network-private','-n','proxy','--timeout=120s')
    api_ip=json.loads(run('get','service','kubernetes','-n','default','-o','json'))['spec']['clusterIP']
    probes={'public':'https://1.1.1.1','private':'http://private.proxy.svc:8080','kubernetes':'https://'+api_ip,'metadata':'http://169.254.169.254','pod-identity':'http://169.254.170.23'}
    script = '''import json,subprocess,time
urls=PROBES
def curl(args):
 r=subprocess.run(['curl','-sS','--connect-timeout','2','--max-time','4','-o','/dev/null','-w','%{http_code}']+args,capture_output=True,text=True)
 return {'exit':r.returncode,'http':r.stdout}
for attempt in range(2):
 result={k:curl(['-k','--noproxy','*',url]) for k,url in urls.items()}
 result['proxy_allowed']=curl(['--proxy','http://proxy.proxy.svc:3128','https://api.anthropic.com'])
 result['proxy_private']=curl(['--proxy','http://proxy.proxy.svc:3128','https://private.proxy.svc:8080'])
 result['proxy_ip']=curl(['--proxy','http://proxy.proxy.svc:3128','https://169.254.169.254'])
 print(json.dumps(result),flush=True)
 if attempt==0: time.sleep(3)
'''.replace('PROBES',repr(probes))
    results={}
    for name, selected in [('network-control',False),('network-fresh-1',True),('network-fresh-2',True)]:
        selected_labels = network_labels if selected else {}
        apply(pod(name,a.namespace,selected_labels,['python3','-c',script]))
        for _ in range(120):
            phase=json.loads(run('get','pod',name,'-n',a.namespace,'-o','json'))['status']['phase']
            if phase in ['Succeeded','Failed']: break
            time.sleep(1)
        if phase!='Succeeded': raise SystemExit(name+' did not complete')
        results[name]=[json.loads(line) for line in run('logs',name,'-n',a.namespace).splitlines()]
    print(json.dumps({'fleetUID':f['metadata']['uid'],'policyDigest':revision,'results':results,'limits':'Metadata timeouts alone are inconclusive; require CNI policy-denied flow evidence before activation.'},indent=2))
    for sample in results['network-control']:
        if any(sample[key]['exit'] != 0 for key in ['public','private','kubernetes','proxy_allowed']):
            raise SystemExit('Positive control failed; denial evidence is inconclusive')
    for name in ['network-fresh-1','network-fresh-2']:
        for sample in results[name]:
            if any(sample[key]['exit'] == 0 for key in probes): raise SystemExit(name+' allowed direct egress')
            if sample['proxy_allowed']['exit']!=0: raise SystemExit(name+' proxy unavailable')
            if any(sample[key]['exit']==0 for key in ['proxy_private','proxy_ip']): raise SystemExit(name+' proxy allowed forbidden destination')
    print('PASS: both fresh Pods deny direct probes and allow only the tested proxy target; review CNI drop evidence separately.')

if __name__=='__main__': main()
