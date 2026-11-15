import {retainArtifacts} from './prune-artifacts.mjs';

// Maintenance failure must not change successful packaging or its stdout.
export function tryRetainArtifacts(keepPaths = []) {
  try { retainArtifacts({keepPaths}); }
  catch (error) { console.warn('Krasis artifact retention deferred: ' + error.message); }
}
