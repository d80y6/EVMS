import React from 'react';
import ReactDOM from 'react-dom/client';
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import { AuthProvider } from './context/AuthContext';
import ProtectedRoute from './components/ProtectedRoute';
import Layout from './components/Layout';
import Dashboard from './components/Dashboard';
import LoginPage from './pages/LoginPage';
import RecordingsPage from './pages/RecordingsPage';
import EventsPage from './pages/EventsPage';
import SettingsPage from './pages/SettingsPage';
import AdminPage from './pages/AdminPage';
import SearchPage from './pages/SearchPage';
import MapPage from './pages/MapPage';
import HealthPage from './pages/HealthPage';
import StoragePage from './pages/StoragePage';
import CamerasPage from './pages/CamerasPage';
import { LegalHoldPage } from './pages/LegalHoldPage';
import BookmarksPage from './pages/BookmarksPage';
import ExportPage from './pages/ExportPage';
import AlertsPage from './pages/AlertsPage';
import AnalyticsPage from './pages/AnalyticsPage';
import AuditPage from './pages/AuditPage';
import POSPage from './pages/POSPage';
import DiscoveryPage from './pages/DiscoveryPage';
import OnvifEventsPage from './pages/OnvifEventsPage';
import OnvifRecordingsPage from './pages/OnvifRecordingsPage';
import ImagingPage from './pages/ImagingPage';
import WebhooksPage from './pages/WebhooksPage';
import DevicePage from './pages/DevicePage';
import MfaPage from './pages/MfaPage';
import EvidencePage from './pages/EvidencePage';
import IncidentsPage from './pages/IncidentsPage';
import ForensicsPage from './pages/ForensicsPage';
import ConfigPage from './pages/ConfigPage';
import RetentionPage from './pages/RetentionPage';
import TimelinePage from './pages/TimelinePage';
import ZonesPage from './pages/ZonesPage';
import ChannelsPage from './pages/ChannelsPage';
import SessionsPage from './pages/SessionsPage';
import SsoPage from './pages/SsoPage';
import CsrfPage from './pages/CsrfPage';
import './index.css';

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <BrowserRouter>
      <AuthProvider>
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route
            path="/"
            element={
              <ProtectedRoute>
                <Layout>
                  <Dashboard />
                </Layout>
              </ProtectedRoute>
            }
          />
          <Route
            path="/recordings"
            element={
              <ProtectedRoute>
                <Layout>
                  <RecordingsPage />
                </Layout>
              </ProtectedRoute>
            }
          />
          <Route
            path="/events"
            element={
              <ProtectedRoute>
                <Layout>
                  <EventsPage />
                </Layout>
              </ProtectedRoute>
            }
          />
          <Route
            path="/admin"
            element={
              <ProtectedRoute>
                <Layout>
                  <AdminPage />
                </Layout>
              </ProtectedRoute>
            }
          />
          <Route
            path="/search"
            element={
              <ProtectedRoute>
                <Layout>
                  <SearchPage />
                </Layout>
              </ProtectedRoute>
            }
          />
          <Route
            path="/settings"
            element={
              <ProtectedRoute>
                <Layout>
                  <SettingsPage />
                </Layout>
              </ProtectedRoute>
            }
          />
          <Route
            path="/map"
            element={
              <ProtectedRoute>
                <Layout>
                  <MapPage />
                </Layout>
              </ProtectedRoute>
            }
          />
          <Route
            path="/health"
            element={
              <ProtectedRoute>
                <Layout>
                  <HealthPage />
                </Layout>
              </ProtectedRoute>
            }
          />
          <Route
            path="/storage"
            element={
              <ProtectedRoute>
                <Layout>
                  <StoragePage />
                </Layout>
              </ProtectedRoute>
            }
          />
          <Route path="/cameras" element={<ProtectedRoute><Layout><CamerasPage /></Layout></ProtectedRoute>} />
          <Route path="/legal-holds" element={<ProtectedRoute><Layout><LegalHoldPage /></Layout></ProtectedRoute>} />
          <Route path="/bookmarks" element={<ProtectedRoute><Layout><BookmarksPage /></Layout></ProtectedRoute>} />
          <Route path="/export" element={<ProtectedRoute><Layout><ExportPage /></Layout></ProtectedRoute>} />
          <Route path="/alerts" element={<ProtectedRoute><Layout><AlertsPage /></Layout></ProtectedRoute>} />
          <Route path="/analytics" element={<ProtectedRoute><Layout><AnalyticsPage /></Layout></ProtectedRoute>} />
          <Route path="/audit" element={<ProtectedRoute><Layout><AuditPage /></Layout></ProtectedRoute>} />
          <Route path="/pos" element={<ProtectedRoute><Layout><POSPage /></Layout></ProtectedRoute>} />
          <Route path="/discovery" element={<ProtectedRoute><Layout><DiscoveryPage /></Layout></ProtectedRoute>} />
          <Route path="/onvif-events" element={<ProtectedRoute><Layout><OnvifEventsPage /></Layout></ProtectedRoute>} />
          <Route path="/onvif-recordings" element={<ProtectedRoute><Layout><OnvifRecordingsPage /></Layout></ProtectedRoute>} />
          <Route path="/imaging" element={<ProtectedRoute><Layout><ImagingPage /></Layout></ProtectedRoute>} />
          <Route path="/webhooks" element={<ProtectedRoute><Layout><WebhooksPage /></Layout></ProtectedRoute>} />
          <Route path="/devices" element={<ProtectedRoute><Layout><DevicePage /></Layout></ProtectedRoute>} />
          <Route path="/mfa" element={<ProtectedRoute><Layout><MfaPage /></Layout></ProtectedRoute>} />
          <Route path="/evidence" element={<ProtectedRoute><Layout><EvidencePage /></Layout></ProtectedRoute>} />
          <Route path="/incidents" element={<ProtectedRoute><Layout><IncidentsPage /></Layout></ProtectedRoute>} />
          <Route path="/forensics" element={<ProtectedRoute><Layout><ForensicsPage /></Layout></ProtectedRoute>} />
          <Route path="/admin/config" element={<ProtectedRoute><Layout><ConfigPage /></Layout></ProtectedRoute>} />
          <Route path="/admin/retention" element={<ProtectedRoute><Layout><RetentionPage /></Layout></ProtectedRoute>} />
          <Route path="/admin/timeline" element={<ProtectedRoute><Layout><TimelinePage /></Layout></ProtectedRoute>} />
          <Route path="/admin/zones" element={<ProtectedRoute><Layout><ZonesPage /></Layout></ProtectedRoute>} />
          <Route path="/admin/channels" element={<ProtectedRoute><Layout><ChannelsPage /></Layout></ProtectedRoute>} />
          <Route path="/admin/sessions" element={<ProtectedRoute><Layout><SessionsPage /></Layout></ProtectedRoute>} />
          <Route path="/admin/sso" element={<ProtectedRoute><Layout><SsoPage /></Layout></ProtectedRoute>} />
          <Route path="/csrf" element={<ProtectedRoute><Layout><CsrfPage /></Layout></ProtectedRoute>} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </AuthProvider>
    </BrowserRouter>
  </React.StrictMode>
);
