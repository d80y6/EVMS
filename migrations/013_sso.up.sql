-- SSO/SAML/OIDC Support
CREATE TABLE IF NOT EXISTS sso_providers (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name TEXT NOT NULL UNIQUE,
    provider_type TEXT NOT NULL CHECK (provider_type IN ('oidc', 'saml')),
    issuer_url TEXT,
    client_id TEXT,
    client_secret_encrypted TEXT,
    scopes TEXT DEFAULT 'openid profile email',
    metadata_url TEXT,
    acs_url TEXT,
    entity_id TEXT,
    certificate TEXT,
    enabled BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS sso_identities (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider_id UUID NOT NULL REFERENCES sso_providers(id) ON DELETE CASCADE,
    external_id TEXT NOT NULL,
    email TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(provider_id, external_id)
);

CREATE INDEX IF NOT EXISTS idx_sso_identities_user ON sso_identities(user_id);
CREATE INDEX IF NOT EXISTS idx_sso_identities_provider ON sso_identities(provider_id);
