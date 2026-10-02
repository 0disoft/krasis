// Only consent and bounded transfer metadata cross the WebView boundary.
export function projectClipboardTransfer(value) {
 if (!value || value.format!=='files' || !['PREPARING','QUEUED','SENDING','SENT','RECEIVING','READY','APPLIED','FAILED'].includes(value.state)) return null;
 const bytes=value.bytes??0,total=value.total??0;
 if (![bytes,total].every(n=>Number.isSafeInteger(n)&&n>=0&&n<=64*1024*1024) || total>0&&bytes>total) return null;
 const code=['size_limit','item_limit','clipboard_changed','prepare_failed','apply_failed','receive_failed','expired','apply_expired'].includes(value.code)?value.code:'';
 return {state:value.state,code,bytes,total};
}
export function createClipboardBridge(binding) {
 async function call(method,...args) {
  try {
   const value=await binding[method](...args);
   if (!value || typeof value.code!=='string' || typeof value.peer!=='string' ||
       (value.peer!=='' && !/^[a-f0-9]{64}$/i.test(value.peer))) throw new Error();
   const transfer=projectClipboardTransfer(value.transfer);
   return {peer:value.peer,code:value.code,...(transfer?{transfer}:{})};
  } catch {return {peer:'',code:'unavailable'};}
 }
 return Object.freeze({
  status:()=>call('ClipboardStatus'),
  set:(peer,enable)=>{
   if (!/^[a-f0-9]{64}$/i.test(peer) || typeof enable!=='boolean') return Promise.resolve({peer:'',code:'invalid_peer'});
   return call('ClipboardSet',peer,enable);
  },
 });
}
