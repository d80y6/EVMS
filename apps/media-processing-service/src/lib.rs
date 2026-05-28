pub mod config;
pub mod error;
pub mod pipeline;
pub mod preprocess;
pub mod inference;
pub mod postprocess;
pub mod api;
pub mod metrics;

pub use config::Config;
pub use error::{MediaProcessingError, Result};
pub use pipeline::{PipelineManager, PipelineConfig, Pipeline, PipelineState, PipelineCommand};
pub use preprocess::{FramePreprocessor, PreprocessConfig, ChannelOrder};
pub use inference::{InferenceClient, InferenceRequest, InferenceResponse, TensorInput, TensorOutput, DataType};
pub use postprocess::{Detection, BoundingBox, non_max_suppression, apply_confidence_threshold, scale_boxes};
