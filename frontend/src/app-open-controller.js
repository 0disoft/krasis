import { appOpenProgress } from './app-open-bridge.js';

const budget = 70000;
const interval = 1500;
const pending = code => ['pending', 'launch_accepted', 'launch_requested'].includes(code);
const remaining = value => Number.isFinite(value) && value >= 0 ? Math.min(budget, value) : budget;

// Cancellation ends this view's observation only. The bridge keeps ownership
// of any outstanding native call and its no-replay fence until it really ends.
export function createAppOpenController(bridge, {
  clock = () => performance.now(),
  scheduler = { setTimeout: (fn, delay) => setTimeout(fn, delay), clearTimeout: id => clearTimeout(id) },
  signal,
  apply = () => {},
} = {}) {
  let active;
  let disposed = false;
  const current = job => !disposed && active === job;
  // Receipts contain scalar fields. Keep callback-owned snapshots separate
  // from the request identity and acknowledgement retained by this controller.
  const publish = (busy, result) => apply({ busy, result: { ...result } });
  function clear(job) {
    scheduler.clearTimeout(job.poll);
    scheduler.clearTimeout(job.expiry);
  }
  function finish(job, result) {
    if (!current(job)) return;
    clear(job);
    active = undefined;
    publish(false, result);
  }
  function expire(job) {
    if (!current(job)) return;
    const left = job.deadline - clock();
    if (left > 0) {
      job.expiry = scheduler.setTimeout(() => expire(job), left);
      return;
    }
    finish(job, appOpenProgress(job.result, { code: 'unavailable' }, true).result);
  }
  function schedule(job) {
    job.poll = scheduler.setTimeout(() => observe(job), Math.min(interval, Math.max(0, job.deadline - clock())));
  }
  function accept(job, status, initial) {
    if (!current(job)) return;
    if (clock() >= job.deadline) { expire(job); return; }
    if (!status || typeof status.code !== 'string') status = { ...job.result, code: initial ? 'outcome_unknown' : 'unavailable' };
    status = { ...status };
    if (initial) {
      job.result = status;
      if (!pending(status.code)) { finish(job, status); return; }
      // Existing receipts may have less time left. A response never renews
      // the bound that already covered the initial native call.
      job.deadline = Math.min(job.deadline, clock() + remaining(status.remaining));
      scheduler.clearTimeout(job.expiry);
      job.expiry = scheduler.setTimeout(() => expire(job), Math.max(0, job.deadline - clock()));
    } else {
      const progress = appOpenProgress(job.result, status, false);
      job.result = progress.result;
      if (!progress.waiting) { finish(job, job.result); return; }
    }
    publish(true, job.result);
    schedule(job);
  }
  function invoke(job, initial) {
    // Handle synchronous adapter failures and late rejection as well as normal
    // promises, without leaving unhandled rejections after disposal/timeout.
    Promise.resolve().then(() => {
      if (!current(job)) return;
      return initial ? bridge.open(job.peer) : bridge.status(job.peer, job.result.id);
    }).then(status => {
      if (current(job)) accept(job, status, initial);
    }, () => {
      if (current(job)) accept(job, { ...job.result, code: initial ? 'outcome_unknown' : 'unavailable' }, initial);
    });
  }
  function observe(job) {
    if (!current(job)) return;
    if (clock() >= job.deadline) { expire(job); return; }
    invoke(job, false);
  }
  function begin(peer, receipt) {
    if (disposed || signal?.aborted || active) return false;
    // A reloaded, locally expired receipt still needs native confirmation.
    // Give that one observation a bounded window; never issue a new launch.
    const left = receipt ? (remaining(receipt.remaining) || budget) : budget;
    const job = { peer, result: receipt ? { ...receipt } : { peer, code: 'pending' }, deadline: clock() + left };
    active = job;
    job.expiry = scheduler.setTimeout(() => expire(job), left);
    publish(true, job.result);
    if (receipt) observe(job);
    else invoke(job, true);
    return true;
  }
  function dispose() {
    if (disposed) return;
    if (active) clear(active);
    active = undefined;
    disposed = true;
    apply = () => {}; // A late native call must not retain this view callback.
    signal?.removeEventListener('abort', abort);
  }
  function abort() {
    if (active) finish(active, appOpenProgress(active.result, { code: 'unavailable' }, true).result);
    dispose();
  }
  signal?.addEventListener('abort', abort, { once: true });
  if (signal?.aborted) dispose();
  return Object.freeze({
    open: peer => begin(peer),
    resume(peer) {
      if (disposed || active) return false;
      const receipt = bridge.resume?.(peer);
      return receipt ? begin(peer, receipt) : false;
    },
    dispose,
  });
}
