import React, { useState, useEffect } from 'react';
import axios from 'axios';
import { AreaChart, Area, LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer } from 'recharts';
import html2pdf from 'html2pdf.js';
import '../styles/Dashboard.css';

export default function Dashboard() {
  const [data, setData] = useState([]);
  const [latest, setLatest] = useState(null);
  const [changes, setChanges] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  useEffect(() => {
    fetchData();
    const interval = setInterval(fetchData, 300000); // Refresh every 5 min
    return () => clearInterval(interval);
  }, []);

  const fetchData = async () => {
    try {
      const [historyRes, latestRes, changesRes] = await Promise.all([
        axios.get('/api/ip-history?limit=500'),
        axios.get('/api/latest'),
        axios.get('/api/ip-changes')
      ]);
      
      setData(historyRes.data);
      setLatest(latestRes.data);
      setChanges(changesRes.data);
      setError(null);
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  const exportPDF = () => {
    const element = document.querySelector('.dashboard-page');
    const opt = {
      margin: 15,
      filename: `homerouter-ip-report-${new Date().toISOString().split('T')[0]}.pdf`,
      image: { type: 'jpeg', quality: 0.98 },
      html2canvas: { scale: 2, backgroundColor: '#f5f7fa' },
      jsPDF: { orientation: 'portrait', unit: 'mm', format: 'a4' }
    };
    html2pdf().set(opt).from(element).save();
  };

  if (loading) {
    return (
      <div className="dashboard-page loading">
        <div className="loader"></div>
        <p>Loading IP tracking data...</p>
      </div>
    );
  }

  return (
    <div className="dashboard-page">
      <header className="page-header">
        <div className="header-content">
          <h1>homerouter-hawk</h1>
          <p className="subtitle">Public IP Address Tracker & History</p>
        </div>
        <button onClick={exportPDF} className="btn-primary">📊 Export Report</button>
      </header>

      {error && <div className="alert alert-error">⚠️ {error}</div>}

      <main className="dashboard-content">
        {latest && (
          <section className="metrics-section">
            <div className="metric-card primary">
              <div className="metric-header">
                <h3>Current IPv4</h3>
                <span className="status-badge active">Active</span>
              </div>
              <div className="metric-value">{latest.ipv4}</div>
              <div className="metric-footer">Updated {new Date(latest.timestamp).toLocaleString('da-DK')}</div>
            </div>

            <div className="metric-card">
              <div className="metric-header">
                <h3>Current IPv6</h3>
              </div>
              {latest.ipv6 && latest.ipv6.includes(':') ? (
                <>
                  <div className="metric-value ipv6">{latest.ipv6}</div>
                  <div className="metric-footer">IPv6 Address</div>
                </>
              ) : (
                <>
                  <div className="metric-placeholder">No IPv6 available</div>
                  <div className="metric-footer">IPv6 not configured</div>
                </>
              )}
            </div>

            <div className="metric-card">
              <div className="metric-header">
                <h3>Total Changes</h3>
              </div>
              <div className="metric-value changes">{changes.length}</div>
              <div className="metric-footer">IP changes detected</div>
            </div>

            <div className="metric-card">
              <div className="metric-header">
                <h3>Tracking Period</h3>
              </div>
              <div className="metric-value">
                {data.length > 1 
                  ? Math.round((new Date(data[data.length - 1]?.timestamp) - new Date(data[0]?.timestamp)) / (1000 * 60 * 60 * 24))
                  : 0
                }
              </div>
              <div className="metric-footer">days of history</div>
            </div>
          </section>
        )}

        <section className="charts-section">
          <div className="chart-card">
            <div className="chart-header">
              <h3>IP Address Timeline</h3>
              <p className="chart-subtitle">Last {Math.min(data.length, 100)} records</p>
            </div>
            <ResponsiveContainer width="100%" height={320}>
              <AreaChart data={data.slice(-100)}>
                <defs>
                  <linearGradient id="ipv4-gradient" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="5%" stopColor="#3b82f6" stopOpacity={0.8}/>
                    <stop offset="95%" stopColor="#3b82f6" stopOpacity={0.1}/>
                  </linearGradient>
                  <linearGradient id="ipv6-gradient" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="5%" stopColor="#8b5cf6" stopOpacity={0.8}/>
                    <stop offset="95%" stopColor="#8b5cf6" stopOpacity={0.1}/>
                  </linearGradient>
                </defs>
                <CartesianGrid strokeDasharray="3 3" stroke="#e5e7eb" />
                <XAxis dataKey="timestamp" stroke="#9ca3af" height={40} />
                <YAxis stroke="#9ca3af" />
                <Tooltip 
                  contentStyle={{ backgroundColor: '#fff', border: '1px solid #e5e7eb', borderRadius: '6px' }}
                  labelStyle={{ color: '#1f2937' }}
                />
                <Area type="monotone" dataKey="ipv4" stroke="#3b82f6" fillOpacity={1} fill="url(#ipv4-gradient)" name="IPv4" />
              </AreaChart>
            </ResponsiveContainer>
          </div>

          <div className="chart-card">
            <div className="chart-header">
              <h3>IP Change Events</h3>
              <p className="chart-subtitle">Last {Math.min(changes.length, 50)} changes</p>
            </div>
            <ResponsiveContainer width="100%" height={320}>
              <LineChart data={changes.slice(-50)}>
                <CartesianGrid strokeDasharray="3 3" stroke="#e5e7eb" />
                <XAxis dataKey="timestamp" stroke="#9ca3af" height={40} />
                <YAxis stroke="#9ca3af" />
                <Tooltip 
                  contentStyle={{ backgroundColor: '#fff', border: '1px solid #e5e7eb', borderRadius: '6px' }}
                  labelStyle={{ color: '#1f2937' }}
                />
                <Legend />
                <Line type="monotone" dataKey="ipv4" stroke="#3b82f6" strokeWidth={2} dot={false} name="IPv4" />
                <Line type="monotone" dataKey="ipv6" stroke="#8b5cf6" strokeWidth={2} dot={false} name="IPv6" />
              </LineChart>
            </ResponsiveContainer>
          </div>
        </section>

        <section className="table-section">
          <div className="section-header">
            <h3>Complete History</h3>
            <p className="section-subtitle">All recorded IP addresses and changes</p>
          </div>
          <div className="table-wrapper">
            <table className="history-table">
              <thead>
                <tr>
                  <th>Date & Time</th>
                  <th>IPv4 Address</th>
                  <th>IPv6 Address</th>
                  <th>Status</th>
                </tr>
              </thead>
              <tbody>
                {data.slice().reverse().slice(0, 100).map((row) => (
                  <tr key={row.id} className={row.change ? 'row-changed' : ''}>
                    <td className="col-timestamp">{new Date(row.timestamp).toLocaleString('da-DK')}</td>
                    <td className="col-ip"><code>{row.ipv4}</code></td>
                    <td className="col-ip">
                      {row.ipv6 && row.ipv6.includes(':') ? (
                        <code>{row.ipv6}</code>
                      ) : (
                        <span className="text-muted">—</span>
                      )}
                    </td>
                    <td className="col-status">
                      {row.change ? (
                        <span className="badge badge-changed">🔄 Changed</span>
                      ) : (
                        <span className="badge badge-stable">✓ Stable</span>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      </main>

      <footer className="page-footer">
        <p>homerouter-hawk • Public IP Tracking Service</p>
        <p className="footer-meta">Last updated: {new Date().toLocaleString('da-DK')}</p>
      </footer>
    </div>
  );
}
