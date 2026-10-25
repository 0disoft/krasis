// Only consent and bounded transfer metadata cross the WebView boundary.
export function projectClipboardTransfer(value) {
 if (!value) return null;
 const format=value.format===undefined?'':value.format;
 if (!['','png','dib','files'].includes(format) || !['PREPARING','QUEUED','SENDING','SENT','RECEIVING','READY','APPLIED','FAILED'].includes(value.state)) return null;
 if (format!=='files' && value.state!=='FAILED') return null;
 const bytes=value.bytes??0,total=value.total??0;
 const limit=format==='files'?64*1024*1024:format===''?64*1024:40*1024*1024;
 if (![bytes,total].every(n=>Number.isSafeInteger(n)&&n>=0&&n<=limit) || total>0&&bytes>total) return null;
 const code=['size_limit','item_limit','clipboard_changed','prepare_failed','apply_failed','receive_failed','expired','apply_expired'].includes(value.code)?value.code:'';
 return {format,state:value.state,code,bytes,total};
}
export function clipboardTransferLabelKey(value) {
 if (value.format==='files') return 'clipboard_files_'+(value.state==='FAILED'&&value.code?value.code:value.state.toLowerCase());
 if (value.code==='size_limit') return value.format===''?'clipboard_text_size_limit':'clipboard_image_size_limit';
 if (['apply_failed','clipboard_changed','expired','apply_expired'].includes(value.code)) return 'clipboard_files_'+value.code;
 return 'clipboard_content_failed';
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
