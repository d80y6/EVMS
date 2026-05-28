use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct CollectionSchema {
    pub name: String,
    pub description: Option<String>,
    pub fields: Vec<FieldSchema>,
    pub index_params: IndexParams,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct FieldSchema {
    pub name: String,
    pub field_type: FieldType,
    pub is_primary: bool,
    pub auto_id: bool,
    pub dimension: Option<i64>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub enum FieldType {
    Int64,
    Float,
    VarChar { max_length: i32 },
    FloatVector { dimension: i64 },
    BinaryVector { dimension: i64 },
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct IndexParams {
    pub index_type: String,
    pub metric_type: String,
    pub params: IndexSpecificParams,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct IndexSpecificParams {
    #[serde(default)]
    pub m: Option<i32>,
    #[serde(default)]
    pub ef_construction: Option<i32>,
    #[serde(default)]
    pub nlist: Option<i32>,
}

impl CollectionSchema {
    pub fn new(name: &str, dimension: i64) -> Self {
        Self {
            name: name.to_string(),
            description: None,
            fields: vec![
                FieldSchema {
                    name: "id".to_string(),
                    field_type: FieldType::Int64,
                    is_primary: true,
                    auto_id: false,
                    dimension: None,
                },
                FieldSchema {
                    name: "embedding".to_string(),
                    field_type: FieldType::FloatVector { dimension },
                    is_primary: false,
                    auto_id: false,
                    dimension: Some(dimension),
                },
            ],
            index_params: IndexParams {
                index_type: "HNSW".to_string(),
                metric_type: "COSINE".to_string(),
                params: IndexSpecificParams {
                    m: Some(16),
                    ef_construction: Some(200),
                    nlist: None,
                },
            },
        }
    }
}
