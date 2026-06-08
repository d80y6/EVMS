import { useState, useEffect } from 'react';
import { api } from '../api/client';

export default function MfaPage() {
  const [enabled, setEnabled] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showEnroll, setShowEnroll] = useState(false);
  const [secret, setSecret] = useState('');
  const [uri, setUri] = useState('');
  const [recoveryCodes, setRecoveryCodes] = useState<string[]>([]);
  const [verifyCode, setVerifyCode] = useState('');
  const [verifying, setVerifying] = useState(false);
  const [enrolled, setEnrolled] = useState(false);

  useEffect(() => {
    api.getMFAStatus()
      .then((data) => setEnabled(data.enabled))
      .catch(() => setError('Failed to load MFA status'))
      .finally(() => setLoading(false));
  }, []);

  const handleEnroll = async () => {
    try {
      setError(null);
      const data = await api.enrollMFA();
      setSecret(data.secret);
      setUri(data.uri);
      setRecoveryCodes(data.recovery_codes);
      setShowEnroll(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to enroll MFA');
    }
  };

  const handleVerify = async () => {
    if (!verifyCode) return;
    setVerifying(true);
    setError(null);
    try {
      await api.verifyMFA(verifyCode);
      setEnabled(true);
      setEnrolled(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Verification failed');
    } finally {
      setVerifying(false);
    }
  };

  const handleDisable = async () => {
    if (!window.confirm('Disable MFA? This will remove all configured authenticator apps.')) return;
    try {
      setError(null);
      await api.verifyMFA('');
      setEnabled(false);
      setShowEnroll(false);
      setEnrolled(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to disable MFA');
    }
  };

  const handleDownloadRecovery = () => {
    const blob = new Blob([recoveryCodes.join('\n')], { type: 'text/plain' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = 'recovery-codes.txt';
    a.click();
    URL.revokeObjectURL(url);
  };

  if (loading) return <div className="p-4 text-slate-400">Loading MFA settings...</div>;

  return (
    <div className="max-w-2xl space-y-6">
      <h2 className="text-lg font-semibold text-slate-200">Multi-Factor Authentication</h2>

      {error && (
        <div className="bg-red-900/20 border border-red-800 rounded-xl p-4">
          <p className="text-sm text-red-400">{error}</p>
        </div>
      )}

      <div className="bg-slate-900 border border-slate-800 rounded-xl p-6 space-y-6">
        <div className="flex items-center justify-between">
          <div>
            <h3 className="text-sm font-medium text-slate-400">MFA Status</h3>
            <p className="text-xs text-slate-500 mt-1">
              {enabled ? 'Authenticator app is configured' : 'No authenticator app configured'}
            </p>
          </div>
          <span className={`px-3 py-1 text-xs font-medium rounded-full ${
            enabled ? 'bg-green-900/30 text-green-400' : 'bg-slate-800 text-slate-400'
          }`}>
            {enabled ? 'Enabled' : 'Disabled'}
          </span>
        </div>

        {!enabled && !showEnroll && (
          <button
            onClick={handleEnroll}
            className="px-4 py-2 bg-indigo-600 hover:bg-indigo-500 text-white text-sm font-medium rounded-lg transition-colors"
          >
            Set Up MFA
          </button>
        )}

        {enabled && !showEnroll && (
          <button
            onClick={handleDisable}
            className="px-4 py-2 bg-red-600 hover:bg-red-500 text-white text-sm font-medium rounded-lg transition-colors"
          >
            Disable MFA
          </button>
        )}

        {showEnroll && !enrolled && (
          <div className="space-y-4 border-t border-slate-800 pt-4">
            <h3 className="text-sm font-medium text-slate-300">Step 1: Scan QR Code</h3>
            <p className="text-xs text-slate-500">
              Scan the QR code with your authenticator app (e.g. Google Authenticator, Authy).
            </p>

            {uri && (
              <div className="bg-slate-800 rounded-lg p-4">
                <p className="text-xs text-slate-400 mb-2">Manual setup URI:</p>
                <code className="text-xs text-indigo-300 break-all select-all">{uri}</code>
              </div>
            )}

            {secret && (
              <div className="bg-slate-800 rounded-lg p-4">
                <p className="text-xs text-slate-400 mb-2">Secret key (manual entry):</p>
                <code className="text-xs text-indigo-300 font-mono select-all">{secret}</code>
              </div>
            )}

            <h3 className="text-sm font-medium text-slate-300 pt-2">Step 2: Verify Code</h3>
            <p className="text-xs text-slate-500">Enter the 6-digit code from your authenticator app.</p>

            <div className="flex items-center gap-3">
              <input
                type="text"
                value={verifyCode}
                onChange={(e) => setVerifyCode(e.target.value.replace(/\D/g, '').slice(0, 6))}
                placeholder="000000"
                maxLength={6}
                className="w-32 bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white text-center font-mono text-lg tracking-widest focus:outline-none focus:ring-2 focus:ring-indigo-500"
              />
              <button
                onClick={handleVerify}
                disabled={verifying || verifyCode.length !== 6}
                className="px-4 py-2 bg-indigo-600 hover:bg-indigo-500 disabled:bg-indigo-800 text-white text-sm font-medium rounded-lg transition-colors"
              >
                {verifying ? 'Verifying...' : 'Verify'}
              </button>
            </div>
          </div>
        )}

        {enrolled && recoveryCodes.length > 0 && (
          <div className="border-t border-slate-800 pt-4 space-y-4">
            <h3 className="text-sm font-medium text-amber-400">Recovery Codes</h3>
            <p className="text-xs text-slate-500">
              Save these recovery codes in a safe place. Each code can only be used once.
            </p>
            <div className="bg-slate-800 rounded-lg p-4 font-mono text-sm text-slate-300 space-y-1">
              {recoveryCodes.map((code, i) => (
                <div key={i}>{code}</div>
              ))}
            </div>
            <button
              onClick={handleDownloadRecovery}
              className="px-4 py-2 bg-slate-700 hover:bg-slate-600 text-white text-sm font-medium rounded-lg transition-colors"
            >
              Download Recovery Codes
            </button>
          </div>
        )}
      </div>
    </div>
  );
}
