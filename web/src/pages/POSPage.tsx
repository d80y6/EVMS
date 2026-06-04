import React, { useState, useEffect } from 'react';
import { api, POSTransaction } from '../api/client';

export default function POSPage() {
  const [transactions, setTransactions] = useState<POSTransaction[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [expanded, setExpanded] = useState<string | null>(null);

  useEffect(() => {
    api.getPOSTransactions({ limit: 100 })
      .then(data => { setTransactions(data.transactions || []); setError(null); })
      .catch(err => setError(err instanceof Error ? err.message : 'Failed to load'))
      .finally(() => setLoading(false));
  }, []);

  if (loading) return <div className="p-4 text-slate-400">Loading POS transactions...</div>;

  return (
    <div className="space-y-6">
      <h2 className="text-lg font-semibold text-slate-200">POS Transactions</h2>

      {error && <div className="bg-red-900/20 border border-red-800 rounded-xl p-4"><p className="text-sm text-red-400">{error}</p></div>}

      <div className="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">
        {transactions.length === 0 && <p className="p-6 text-sm text-slate-500">No transactions.</p>}
        {transactions.length > 0 && (
          <table className="w-full text-sm">
            <thead>
              <tr className="text-slate-400 border-b border-slate-800 text-left">
                <th className="p-3">Register</th><th className="p-3">Transaction #</th><th className="p-3">Time</th><th className="p-3">Total</th><th className="p-3">Tender</th><th className="p-3"></th>
              </tr>
            </thead>
            <tbody>
              {transactions.map(tx => (
                <React.Fragment key={tx.id}>
                  <tr className="border-b border-slate-800 hover:bg-slate-800/50 cursor-pointer" onClick={() => setExpanded(expanded === tx.id ? null : tx.id)}>
                    <td className="p-3 text-slate-300">{tx.register_id}</td>
                    <td className="p-3 text-slate-300">#{tx.transaction_id}</td>
                    <td className="p-3 text-slate-300 text-xs">{new Date(tx.timestamp).toLocaleString()}</td>
                    <td className="p-3 text-slate-300 font-medium">${tx.total.toFixed(2)}</td>
                    <td className="p-3 text-slate-300">{tx.tender_type}</td>
                    <td className="p-3 text-slate-500 text-xs">{expanded === tx.id ? '▲' : '▼'}</td>
                  </tr>
                  {expanded === tx.id && (
                    <tr>
                      <td colSpan={6} className="bg-slate-800/50 p-3">
                        <table className="w-full text-xs">
                          <thead><tr className="text-slate-500 border-b border-slate-700">
                            <th className="text-left p-1">Item</th><th className="text-right p-1">Qty</th><th className="text-right p-1">Price</th><th className="text-right p-1">Total</th>
                          </tr></thead>
                          <tbody>
                            {tx.items.map((item, i) => (
                              <tr key={i} className="border-b border-slate-700/50">
                                <td className="p-1 text-slate-300">{item.description}</td>
                                <td className="p-1 text-right text-slate-300">x{item.quantity}</td>
                                <td className="p-1 text-right text-slate-300">${item.unit_price.toFixed(2)}</td>
                                <td className="p-1 text-right text-slate-300">${item.total.toFixed(2)}</td>
                              </tr>
                            ))}
                          </tbody>
                          <tfoot>
                            <tr><td colSpan={3} className="text-right p-1 text-slate-500">Subtotal:</td><td className="text-right p-1 text-slate-300">${tx.subtotal.toFixed(2)}</td></tr>
                            <tr><td colSpan={3} className="text-right p-1 text-slate-500">Tax:</td><td className="text-right p-1 text-slate-300">${tx.tax.toFixed(2)}</td></tr>
                            <tr><td colSpan={3} className="text-right p-1 text-slate-300 font-medium">Total:</td><td className="text-right p-1 text-slate-300 font-medium">${tx.total.toFixed(2)}</td></tr>
                          </tfoot>
                        </table>
                      </td>
                    </tr>
                  )}
                </React.Fragment>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}
