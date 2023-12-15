use serde::{Deserialize, Serialize};

pub(crate) mod tag;
mod kb_info;
mod dialog_info;

#[derive(Debug, Deserialize, Serialize)]
struct JsonResponse<T> {
    code: u32,
    err: String,
    data: T,
}