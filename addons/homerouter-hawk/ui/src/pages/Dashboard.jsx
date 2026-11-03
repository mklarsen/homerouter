import React, { useState, useEffect } from 'react';
import axios from 'axios';
import { LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer } from 'recharts';
import html2pdf from 'html2pdf.js';
import './styles/Dashboard.css';

export default function Dashboard() {
  const [data, setData] = useState([]);
  const [latest, setLatest] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  useEffect(() => {
    fetchData();
    const interval = setInterval(fetchData, 60000); // Refresh every min
    return () => clearInterval(interval);
  }, []);

  const fetchData = async () => {
    try {
      const [historyRes, latestRes] = await Promise.all([
        axios.get('/api/ip-history?limit=500'),
        axios.get('/api/latest')
      ]);
      
      setData(historyRes.data);
      setLatest(latestRes.data);
      setError(null);
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  const exportPDF = () => {
    const element = document.getElementById('dashboard');
    const opt = {
      margin: 10,
      filename: `ip-history-${new Date().toISOString().split('T')[0]}.pdf`,
      image: { type: 'jpeg', quality: 0.98 },
      html2canvas: { scale: 2 },
      jsPDF: { orientation: 'portrait', unit: 'mm', format: 'a4' }
    };
    html2pdf().set(opt).from(element).save();
  };

  if (loading) return <div className="container"><p>Loading...</p></div>;
  if (error) return <div className="container"><p className="error">Error: {error}</p></div>;

  return (
    <div className="container">
      <div id="dashboard">
        <h1>🔍 homerouter-hawk</h1>
        <h2>WAN IP Address Tracker</h2>

        {latest && (
          <div className="card latest">
            <h3>Current Status</h3>
            <div className="ip-display">
              <div>
                <strong>IPv4:</strong> <code>{latest.ipv4}</code>
              </div>
              <div>
                <strong>IPv6:</strong> <code>{latest.ipv6}</code>
              </div>
              <div className="timestamp">
                Last updated: {new Date(latest.timestamp).toLocaleString()}
              </div>
            </div>
          </div>
        )}

        <div className="card chart-container">
          <h3>IP History (Last 500 entries)</h3>
          <ResponsiveContainer width="100%" height={400}>
            <LineChart data={data}>
              <CartesianGrid strokeDasharray="3 3" stroke="#444" />
              <XAxis 
                dataKey="timestamp" 
                tick={{ fontSize: 12 }}
                interval={Math.floor(data.length / 10)}
              />
              <YAxis hide />
              <Tooltip 
                contentStyle={{ backgroundColor: '#1a1a1a', border: '1px solid #444' }}
                formatter={(value) => value}
              />
              <Legend />
              <Line 
                type="stepAfter" 
                dataKey="ipv4" 
                stroke="#00ff41" 
                dot={false}
                isAnimationActive={false}
              />
              <Line 
                type="stepAfter" 
                dataKey="ipv6" 
                stroke="#ff00ff" 
                dot={false}
                isAnimationActive={false}
              />
            </LineChart>
          </ResponsiveContainer>
        </div>

        <div className="card table-container">
          <h3>Detailed History</h3>
          <table>
            <thead>
              <tr>
                <th>Timestamp</th>
                <th>IPv4</th>
                <th>IPv6</th>
                <th>Change</th>
              </tr>
            </thead>
            <tbody>
              {data.slice(-50).reverse().map((row) => (
                <tr key={row.id} className={row.change ? 'changed' : ''}>
                  <td>{new Date(row.timestamp).toLocaleString()}</td>
                  <td><code>{row.ipv4}</code></td>
                  <td><code>{row.ipv6}</code></td>
                  <td>{row.change ? '🔄' : '✓'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      <button onClick={exportPDF} className="btn-export">
        📄 Export PDF
      </button>
    </div>
  );
}
