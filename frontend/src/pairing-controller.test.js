import test from 'node:test';
import assert from 'node:assert/strict';
import {createPairingController} from './pairing-controller.js';
import {createOperationGate} from './operation-gate.js';

const status = (code = 'ok', candidates = []) => ({schema:'krasis.pairing/v1',code,pin:'',job:'',alias:'',candidates});
function deferred() {
  let resolve;
  const promise = new Promise(done => { resolve = done; });
  return {promise,resolve};
}
function setup(bridge) {
  let view = {pairing:status(), notice:'', candidate:'first', pin:'123456'}, active = true, refreshes = 0;
  const visibleBusy = [], applied = [];
  const controller = createPairingController(bridge, createOperationGate(value => visibleBusy.push(value)), {
    active: () => active,
    read: () => view,
    apply: value => { view=value; applied.push(value); },
    refresh: async () => { ++refreshes; },
  });
  return {controller, visibleBusy, applied, get view() {return view;}, get refreshes() {return refreshes;},
    edit: value => {view={...view,...value};}, dispose: () => {active=false;}};
}

test('a join queued behind a poll retains clicked credentials without restoring old form state', async () => {
  const pending = deferred(), calls = [];
  const s = setup({
    status: () => pending.promise,
    join: async (...args) => { calls.push(args); return status('pairing_completed'); },
  });
  const poll = s.controller.run('status', true);
  s.edit({candidate:'clicked',pin:'654321'});
  const join = s.controller.run('join');
  s.edit({candidate:'later',pin:'999999'});
  await s.controller.run('join');
  assert.deepEqual(s.visibleBusy,[true]);
  pending.resolve(status());
  await poll;
  await join;
  assert.deepEqual(calls,[['clicked','654321']]);
  assert.equal(s.view.candidate,'later');
  assert.equal(s.view.pin,'');
  assert.equal(s.view.notice,'pairing_completed');
  assert.equal(s.refreshes,1);
  assert.deepEqual(s.visibleBusy,[true,false]);
});

test('unavailable reads retain an active pairing job and busy results retain its observation', async () => {
  let next = status('unavailable');
  const s = setup({status:async()=>next});
  s.edit({pairing:{...status('pin_waiting'),job:'active-job',pin:'654321'},notice:'retained'});
  await s.controller.run('status',true);
  assert.equal(s.view.pairing.job,'active-job');
  assert.equal(s.view.pairing.pin,'');
  assert.equal(s.view.notice,'retained');
  const observed = s.view.pairing;
  next=status('busy');
  await s.controller.run('status',true);
  assert.equal(s.view.pairing,observed);
  assert.deepEqual(s.visibleBusy,[]);
});

test('search selects a valid candidate while preserving edits made during the query', async () => {
  const pending = deferred();
  const s = setup({search:()=>pending.promise});
  s.edit({notice:'old-notice'});
  const search=s.controller.run('search');
  assert.equal(s.view.notice,'');
  s.edit({candidate:'selected',pin:'654321'});
  pending.resolve(status('ok',[{id:'other'},{id:'selected'}]));
  await search;
  assert.equal(s.view.candidate,'selected');
  assert.equal(s.view.pin,'654321');
  const replacement=setup({search:async()=>status('ok',[{id:'new'}])});
  await replacement.controller.run('search');
  assert.equal(replacement.view.candidate,'new');
});

test('disposing during a join discards its result and never refreshes or repeats the request', async () => {
  const pending=deferred();
  let joins=0;
  const s=setup({join:()=>{++joins;return pending.promise;}});
  const join=s.controller.run('join');
  const appliedBefore=s.applied.length;
  s.dispose();
  pending.resolve(status('paired_connect_failed'));
  await join;
  await s.controller.run('join');
  assert.equal(joins,1);
  assert.equal(s.applied.length,appliedBefore);
  assert.equal(s.refreshes,0);
});
