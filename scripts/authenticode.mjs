import path from 'node:path';
import { execFileSync } from 'node:child_process';

export const signingEnabled = () => Boolean(process.env.KRASIS_SIGN_CERT_FILE);
// No private key or password is passed as a command-line argument or logged.
// This hook is inert for local development builds without a signing identity.
export function signExecutable(file) {
  if (!signingEnabled()) return false;
  const cert = process.env.KRASIS_SIGN_CERT_FILE;
  const thumbprint = process.env.KRASIS_SIGN_CERT_THUMBPRINT;
  const timestamp = process.env.KRASIS_SIGN_TIMESTAMP_URL;
  if (!path.isAbsolute(cert) || !/^[A-Fa-f0-9]{40}$/u.test(thumbprint ?? '') || !/^https:\/\/[^\s]+$/u.test(timestamp ?? '')) throw Error('Authenticode signing configuration incomplete');
  const script = String.raw`
$ErrorActionPreference='Stop'
try {
  $krasisCertificate=[Security.Cryptography.X509Certificates.X509Certificate2]::new($env:KRASIS_SIGN_CERT_FILE,$env:KRASIS_SIGN_CERT_PASSWORD,[Security.Cryptography.X509Certificates.X509KeyStorageFlags]::EphemeralKeySet)
  try {
    if (-not $krasisCertificate.HasPrivateKey -or $krasisCertificate.Thumbprint -ne $env:KRASIS_SIGN_CERT_THUMBPRINT) { exit 2 }
    $krasisSignature=Set-AuthenticodeSignature -LiteralPath $env:KRASIS_SIGN_IMAGE -Certificate $krasisCertificate -HashAlgorithm SHA256 -TimestampServer $env:KRASIS_SIGN_TIMESTAMP_URL
    $krasisVerified=Get-AuthenticodeSignature -LiteralPath $env:KRASIS_SIGN_IMAGE
    if ($krasisSignature.Status -ne 'Valid' -or $krasisVerified.Status -ne 'Valid' -or $krasisVerified.SignerCertificate.Thumbprint -ne $env:KRASIS_SIGN_CERT_THUMBPRINT -or -not $krasisVerified.TimeStamperCertificate) { exit 3 }
  } finally { $krasisCertificate.Dispose() }
} catch { exit 1 }
`;
  // Capture output so crypto/provider errors cannot disclose signing secrets.
  try {
    execFileSync('pwsh', ['-NoProfile','-NonInteractive','-EncodedCommand',Buffer.from(script,'utf16le').toString('base64')],
      {env:{...process.env,KRASIS_SIGN_IMAGE:path.resolve(file)},stdio:'pipe',timeout:120000,windowsHide:true});
  } catch { throw Error('Authenticode signing or verification failed'); }
  return true;
}
