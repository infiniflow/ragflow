use serde::{Deserialize, Serialize};

pub(crate) mod tag;

#[derive(Debug, Deserialize, Serialize)]
struct JsonResponse<T> {
    code: u32,
    err: String,
    data: T,
}