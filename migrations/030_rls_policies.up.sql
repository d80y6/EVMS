-- RLS Policies for multi-tenant isolation
-- Uses current_setting('app.tenant_id', true) which returns NULL when not set,
-- allowing superadmin queries without tenant context to still work.

-- ============================================================
-- Direct tenant_id tables
-- ============================================================

ALTER TABLE users ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON users FOR ALL
    USING (tenant_id = current_setting('app.tenant_id', true)::UUID);

ALTER TABLE sites ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON sites FOR ALL
    USING (tenant_id = current_setting('app.tenant_id', true)::UUID);

ALTER TABLE rules ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON rules FOR ALL
    USING (tenant_id = current_setting('app.tenant_id', true)::UUID);

ALTER TABLE alert_rules ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON alert_rules FOR ALL
    USING (tenant_id = current_setting('app.tenant_id', true)::UUID);

ALTER TABLE tours ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON tours FOR ALL
    USING (tenant_id = current_setting('app.tenant_id', true)::UUID);

ALTER TABLE alerts ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON alerts FOR ALL
    USING (tenant_id = current_setting('app.tenant_id', true)::UUID);

ALTER TABLE incidents ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON incidents FOR ALL
    USING (tenant_id = current_setting('app.tenant_id', true)::UUID);

ALTER TABLE evidence_cases ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON evidence_cases FOR ALL
    USING (tenant_id = current_setting('app.tenant_id', true)::UUID);

ALTER TABLE notification_channels ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON notification_channels FOR ALL
    USING (tenant_id = current_setting('app.tenant_id', true)::UUID);

-- ============================================================
-- Join-based tables
-- ============================================================

-- api_keys: tenant via users
ALTER TABLE api_keys ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON api_keys FOR ALL
    USING (user_id IN (SELECT id FROM users WHERE tenant_id = current_setting('app.tenant_id', true)::UUID));

-- cameras: tenant via sites
ALTER TABLE cameras ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON cameras FOR ALL
    USING (site_id IN (SELECT id FROM sites WHERE tenant_id = current_setting('app.tenant_id', true)::UUID));

-- recordings: tenant via cameras -> sites
ALTER TABLE recordings ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON recordings FOR ALL
    USING (camera_id IN (SELECT id FROM cameras WHERE site_id IN (SELECT id FROM sites WHERE tenant_id = current_setting('app.tenant_id', true)::UUID)));

-- ai_events: tenant via cameras -> sites
ALTER TABLE ai_events ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON ai_events FOR ALL
    USING (camera_id IN (SELECT id FROM cameras WHERE site_id IN (SELECT id FROM sites WHERE tenant_id = current_setting('app.tenant_id', true)::UUID)));

-- bookmarks: tenant via cameras -> sites
ALTER TABLE bookmarks ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON bookmarks FOR ALL
    USING (camera_id IN (SELECT id FROM cameras WHERE site_id IN (SELECT id FROM sites WHERE tenant_id = current_setting('app.tenant_id', true)::UUID)));

-- legal_holds: tenant via cameras -> sites
ALTER TABLE legal_holds ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON legal_holds FOR ALL
    USING (camera_id IN (SELECT id FROM cameras WHERE site_id IN (SELECT id FROM sites WHERE tenant_id = current_setting('app.tenant_id', true)::UUID)));

-- crowd_heatmaps: tenant via cameras -> sites
ALTER TABLE crowd_heatmaps ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON crowd_heatmaps FOR ALL
    USING (camera_id IN (SELECT id FROM cameras WHERE site_id IN (SELECT id FROM sites WHERE tenant_id = current_setting('app.tenant_id', true)::UUID)));

-- ============================================================
-- Child tables of tenant-scoped parents
-- ============================================================

-- incident_events: tenant via incidents
ALTER TABLE incident_events ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON incident_events FOR ALL
    USING (incident_id IN (SELECT id FROM incidents WHERE tenant_id = current_setting('app.tenant_id', true)::UUID));

-- incident_notes: tenant via incidents
ALTER TABLE incident_notes ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON incident_notes FOR ALL
    USING (incident_id IN (SELECT id FROM incidents WHERE tenant_id = current_setting('app.tenant_id', true)::UUID));

-- evidence_lockers: tenant via evidence_cases
ALTER TABLE evidence_lockers ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON evidence_lockers FOR ALL
    USING (case_id IN (SELECT id FROM evidence_cases WHERE tenant_id = current_setting('app.tenant_id', true)::UUID));

-- evidence_items: tenant via evidence_lockers -> evidence_cases
ALTER TABLE evidence_items ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON evidence_items FOR ALL
    USING (locker_id IN (SELECT el.id FROM evidence_lockers el JOIN evidence_cases ec ON el.case_id = ec.id WHERE ec.tenant_id = current_setting('app.tenant_id', true)::UUID));

-- evidence_shares: tenant via evidence_items -> evidence_lockers -> evidence_cases
ALTER TABLE evidence_shares ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON evidence_shares FOR ALL
    USING (item_id IN (SELECT ei.id FROM evidence_items ei JOIN evidence_lockers el ON ei.locker_id = el.id JOIN evidence_cases ec ON el.case_id = ec.id WHERE ec.tenant_id = current_setting('app.tenant_id', true)::UUID));

-- evidence_access_log: tenant via evidence_items -> evidence_lockers -> evidence_cases
ALTER TABLE evidence_access_log ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON evidence_access_log FOR ALL
    USING (item_id IN (SELECT ei.id FROM evidence_items ei JOIN evidence_lockers el ON ei.locker_id = el.id JOIN evidence_cases ec ON el.case_id = ec.id WHERE ec.tenant_id = current_setting('app.tenant_id', true)::UUID));

-- notification_log: tenant via notification_channels
ALTER TABLE notification_log ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON notification_log FOR ALL
    USING (channel_id IN (SELECT id FROM notification_channels WHERE tenant_id = current_setting('app.tenant_id', true)::UUID));
