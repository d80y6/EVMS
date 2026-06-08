use std::collections::HashMap;
use std::sync::Arc;
use tokio::sync::RwLock;
use crate::client::ModelMetadata;

/// Model registry with hot-reloading support
#[derive(Clone)]
pub struct ModelRegistry {
    models: Arc<RwLock<HashMap<String, RegisteredModel>>>,
}

#[derive(Debug, Clone)]
pub struct RegisteredModel {
    pub name: String,
    pub versions: Vec<String>,
    pub metadata: Option<ModelMetadata>,
    pub config: ModelConfig,
    pub ready: bool,
}

#[derive(Debug, Clone, Default)]
pub struct ModelConfig {
    pub max_batch_size: usize,
    pub instance_groups: Vec<InstanceGroup>,
    pub dynamic_batching: bool,
    pub sequence_batching: bool,
}

#[derive(Debug, Clone)]
pub struct InstanceGroup {
    pub kind: InstanceKind,
    pub count: u32,
}

#[derive(Debug, Clone, PartialEq)]
pub enum InstanceKind {
    Cpu,
    Gpu(u32), // GPU ID
}

impl ModelRegistry {
    pub fn new() -> Self {
        Self {
            models: Arc::new(RwLock::new(HashMap::new())),
        }
    }

    pub async fn register(&self, model: RegisteredModel) {
        let mut models = self.models.write().await;
        models.insert(model.name.clone(), model);
    }

    pub async fn unregister(&self, name: &str) -> Option<RegisteredModel> {
        let mut models = self.models.write().await;
        models.remove(name)
    }

    pub async fn get(&self, name: &str) -> Option<RegisteredModel> {
        let models = self.models.read().await;
        models.get(name).cloned()
    }

    pub async fn list_all(&self) -> Vec<RegisteredModel> {
        let models = self.models.read().await;
        models.values().cloned().collect()
    }

    pub async fn is_ready(&self, name: &str) -> bool {
        let models = self.models.read().await;
        models.get(name).map(|m| m.ready).unwrap_or(false)
    }

    pub async fn update_status(&self, name: &str, ready: bool) {
        let mut models = self.models.write().await;
        if let Some(model) = models.get_mut(name) {
            model.ready = ready;
        }
    }

    pub async fn reload(&self, name: &str) -> Result<(), String> {
        // In production, this would reload model from repository
        self.update_status(name, true).await;
        Ok(())
    }
}

impl Default for ModelRegistry {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn test_model_registration() {
        let registry = ModelRegistry::new();
        
        let model = RegisteredModel {
            name: "test-model".to_string(),
            versions: vec!["1".to_string()],
            metadata: None,
            config: ModelConfig::default(),
            ready: true,
        };
        
        registry.register(model).await;
        
        let retrieved = registry.get("test-model").await;
        assert!(retrieved.is_some());
        assert_eq!(retrieved.unwrap().name, "test-model");
    }

    #[tokio::test]
    async fn test_model_list() {
        let registry = ModelRegistry::new();
        
        for i in 0..3 {
            let model = RegisteredModel {
                name: format!("model-{}", i),
                versions: vec!["1".to_string()],
                metadata: None,
                config: ModelConfig::default(),
                ready: true,
            };
            registry.register(model).await;
        }
        
        let all = registry.list_all().await;
        assert_eq!(all.len(), 3);
    }

    #[tokio::test]
    async fn test_model_unregistration() {
        let registry = ModelRegistry::new();
        
        let model = RegisteredModel {
            name: "temp-model".to_string(),
            versions: vec!["1".to_string()],
            metadata: None,
            config: ModelConfig::default(),
            ready: false,
        };
        
        registry.register(model).await;
        assert!(registry.get("temp-model").await.is_some());
        
        let removed = registry.unregister("temp-model").await;
        assert!(removed.is_some());
        assert!(registry.get("temp-model").await.is_none());
    }
}
