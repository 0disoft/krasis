// Only consent/status crosses the WebView boundary. Text is native-owned.
export function createClipboardBridge(binding) {
 async function call(method,...args) {
  try {
   const value=await binding[method](...args);
   if (!value || typeof value.code!=='string' || typeof value.peer!=='string' ||
       (value.peer!=='' && !/^[a-f0-9]{64}$/i.test(value.peer))) throw new Error();
   return {peer:value.peer,code:value.code};
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
