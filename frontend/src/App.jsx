import { useState } from 'react';
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom';
import './App.css';
import { DEFAULT_SCORER } from './constants';
import { Sidebar, Topbar } from './components/layout';
import { CaseDetails, CasesQueue, Dashboard, ModelComparison } from './pages';

function App() {
  const [scorerKey, setScorerKey] = useState(DEFAULT_SCORER);

  return (
    <BrowserRouter>
      <div className="app-shell">
        <Sidebar />
        <main className="main">
          <Topbar scorerKey={scorerKey} setScorerKey={setScorerKey} />
          <Routes>
            <Route path="/" element={<Navigate to="/cases" replace />} />
            <Route path="/cases" element={<CasesQueue scorerKey={scorerKey} />} />
            <Route path="/cases/:id" element={<CaseDetails scorerKey={scorerKey} setScorerKey={setScorerKey} />} />
            <Route path="/dashboard" element={<Dashboard />} />
            <Route path="/evaluation" element={<ModelComparison />} />
          </Routes>
        </main>
      </div>
    </BrowserRouter>
  );
}

export default App;
