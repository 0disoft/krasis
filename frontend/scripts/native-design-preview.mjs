// Local visual QA only. This server is not packaged, talks to no agent, and
// substitutes the Wails module with deterministic, in-memory display fixtures.
import { createServer } from 'node:http';
import { readFile } from 'node:fs/promises';
import { resolve, extname, sep } from 'node:path';
const root = resolve('dist-native');
const mime = {'.html':'text/html; charset=utf-8','.js':'text/javascript','.css':'text/css','.svg':'image/svg+xml','.png':'image/png','.ico':'image/x-icon'};
const fixture = `
const peer='a'.repeat(64), build='b'.repeat(40);
const params=new URLSearchParams(location.search);
let state=params.get('state')==='LOCAL'?'LOCAL':'CONNECTED';
let clipboardPeer=params.get('clipboard')==='on'?peer:'';
const clipboardAvailable=params.get('clipboard')!=='unknown';
const alias=params.get('long')==='1'?'오른쪽 노트북 · 아주 긴 기기 이름 테스트':'B';
const snapshot=()=>({schema:'krasis.desktop/v1',state,peerAlias:state==='CONNECTED'?alias:'',peers:[{alias,fingerprint:peer}],code:'ok',notice:''});
export const Call={ByName:async(name,...args)=>{
 const method=name.split('.').at(-1);
 switch(method){
 case 'Refresh':return snapshot();
 case 'Disconnect':state='LOCAL';return snapshot();
 case 'Connect':state='CONNECTED';return snapshot();
 case 'ClipboardStatus':return clipboardAvailable?{peer:clipboardPeer,code:'ok'}:{peer:'',code:'unavailable'};
 case 'ClipboardSet':
  if(!clipboardAvailable)return {peer:'',code:'unavailable'};
  if(params.get('clipboard')==='reject')return {peer:'',code:'rejected'};
  clipboardPeer=args[1]?args[0]:'';
  return {peer:clipboardPeer,code:'ok'};
 case 'PairStatus':return {schema:'krasis.pairing/v1',code:'ok',pin:'',job:'',alias:'',candidates:[]};
 case 'LayoutStatus':return {edge:'RIGHT',rearm:32,code:'ok'};
 case 'DiagnosticsStatus':return {schema:'krasis.diagnostics/v1',code:'ok',saved:true,received:false,transfer:'NONE'};
 case 'UpdateSettings':return {schema:'krasis.update-settings/v1',code:'ok',peer,permission:'ALLOWED',remotePermission:'ALLOWED',peerState:'ONLINE',localGeneration:36,localBuild:build,peerGeneration:35,peerBuild:'c'.repeat(40),comparison:'LOCAL_NEWER'};
 case 'UpdateTransfer':return {schema:'krasis.update-transfer/v1',peer,state:params.get('job')==='SENDING'?'SENDING':'AGENT_CHANGED',id:'d'.repeat(32),bytes:42,total:100,generation:36,build};
 default:throw Error('Visual fixture does not execute this operation');
 }
}};`;
const server = createServer(async(req,res)=>{
 const url=new URL(req.url,'http://127.0.0.1');
 if(req.method!=='GET'){res.writeHead(405).end();return;}
 if(url.pathname==='/__preview_stop'){
  res.writeHead(200,{'Content-Type':'text/plain; charset=utf-8'}).end('Visual QA stopped');server.close();server.closeIdleConnections();return;
 }
 if(url.pathname==='/wails/runtime.js'){
  res.writeHead(200,{'Content-Type':'text/javascript','Cache-Control':'no-store'}).end(fixture);return;
 }
 try{
  const file=resolve(root,'.'+(url.pathname==='/'?'/index.html':decodeURIComponent(url.pathname)));
  if(!file.startsWith(root+sep)){res.writeHead(403).end();return;}
  const content=await readFile(file);
  res.writeHead(200,{'Content-Type':mime[extname(file)]??'application/octet-stream','Cache-Control':'no-store'}).end(content);
 }catch{res.writeHead(404).end();}
});
server.listen(4181,'127.0.0.1',()=>console.log('Native layout fixtures: http://127.0.0.1:4181 (no live agent; 10 minute limit)'));
const timer=setTimeout(()=>{server.close();server.closeAllConnections();},600000);
server.on('close',()=>clearTimeout(timer));
