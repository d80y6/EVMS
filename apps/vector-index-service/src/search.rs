use crate::error::{Error, Result};

#[derive(Debug, Clone)]
pub struct SearchParams {
    pub k: usize,
    pub metric_type: MetricType,
    pub filter: Option<String>,
}

#[derive(Debug, Clone, Default)]
pub enum MetricType {
    #[default]
    Cosine,
    Euclidean,
    DotProduct,
}

pub struct SearchEngine;

impl SearchEngine {
    pub fn new() -> Self {
        Self
    }

    pub fn knn_search(&self, vectors: &[Vec<f32>], query: &[f32], k: usize) -> Result<Vec<(usize, f32)>> {
        let mut scores: Vec<(usize, f32)> = vectors.iter()
            .enumerate()
            .map(|(i, v)| (i, cosine_similarity(v, query)))
            .collect();
        
        scores.sort_by(|a, b| b.1.partial_cmp(&a.1).unwrap_or(std::cmp::Ordering::Equal));
        Ok(scores.into_iter().take(k).collect())
    }

    pub fn range_search(&self, vectors: &[Vec<f32>], query: &[f32], radius: f32) -> Result<Vec<usize>> {
        let mut results = Vec::new();
        for (i, v) in vectors.iter().enumerate() {
            let similarity = cosine_similarity(v, query);
            if similarity >= radius {
                results.push(i);
            }
        }
        Ok(results)
    }
}

fn cosine_similarity(a: &[f32], b: &[f32]) -> f32 {
    if a.len() != b.len() {
        return 0.0;
    }
    
    let dot: f32 = a.iter().zip(b.iter()).map(|(x, y)| x * y).sum();
    let norm_a: f32 = a.iter().map(|x| x * x).sum::<f32>().sqrt();
    let norm_b: f32 = b.iter().map(|x| x * x).sum::<f32>().sqrt();
    
    if norm_a == 0.0 || norm_b == 0.0 {
        0.0
    } else {
        dot / (norm_a * norm_b)
    }
}
