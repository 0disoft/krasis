import test from 'node:test';
import assert from 'node:assert/strict';
import {createUIReplacementBridge,emptyUIReplacement} from './ui-replacement-bridge.js';
const failed = {schema:'krasis.ui-replacement/v1',state:'FAILED',generation:223,canRetry:true};
const observedFailed = {...failed,fresh:true};
test('replacement status exposes only bounded state and retry never accepts a path',async()=>{
  let args;
  const bridge=createUIReplacementBridge({
    UIReplacementStatus:()=>({...failed,source:'private path',error:'private error'}),
    RetryUIReplacement:(...value)=>{args=value;return {...failed,state:'PREPARING',canRetry:false};}
  });
  assert.deepEqual(await bridge.status(),observedFailed);
  assert.equal((await bridge.retry()).state,'PREPARING');
  assert.deepEqual(args,[]);
  for (const value of [null,{...failed,state:'private'}, {...failed,generation:-1},{...failed,canRetry:'true'},{...failed,state:'APPLYING'}]) {
    assert.deepEqual(await createUIReplacementBridge({UIReplacementStatus:()=>value}).status(),emptyUIReplacement());
  }
});
test('late reads cannot overwrite retry acknowledgement and duplicate retries never replay',async()=>{
  let readDone, retryDone, retries=0, reads=0;
  const bridge=createUIReplacementBridge({
    UIReplacementStatus:()=> ++reads===1 ? failed : new Promise(resolve=>readDone=resolve),
    RetryUIReplacement:()=>{retries++;return new Promise(resolve=>retryDone=resolve);}
  });
  await bridge.status();
  const read=bridge.status(), retry=bridge.retry();
  await bridge.retry();
  await bridge.status();
  assert.equal(retries,1);
  assert.equal(reads,2);
  retryDone({...failed,state:'PREPARING',canRetry:false});
  assert.equal((await retry).state,'PREPARING');
  readDone(failed);
  assert.equal((await read).state,'PREPARING');
});
test('uncertain retry results require fresh status and are never automatically retried',async()=>{
  let retries=0;
  const bridge=createUIReplacementBridge({UIReplacementStatus:()=>failed,RetryUIReplacement:()=>{retries++;throw Error('private failure');}});
  await bridge.status();
  assert.deepEqual(await bridge.retry(),{...failed,canRetry:false,fresh:false});
  await bridge.retry();
  assert.equal(retries,1);
  assert.deepEqual(await bridge.status(),observedFailed);
});

test('transient or malformed reads retain the last observation but disable stale retries',async()=>{
  let next=failed;
  const bridge=createUIReplacementBridge({UIReplacementStatus:()=>{if(next instanceof Error)throw next;return next;}});
  assert.deepEqual(await bridge.status(),observedFailed);
  for(const value of [new Error('private failure'),null,{...failed,canRetry:'true'}]) {
    next=value;
    assert.deepEqual(await bridge.status(),{...failed,canRetry:false,fresh:false});
    assert.equal((await bridge.retry()).fresh,false);
  }
  next=failed;
  assert.deepEqual(await bridge.status(),observedFailed);
  next={schema:failed.schema,state:'IDLE',generation:0,canRetry:false};
  assert.equal((await bridge.status()).state,'IDLE');
});
