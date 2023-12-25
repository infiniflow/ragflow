use serde::{ Deserialize, Serialize };

pub(crate) mod tag_info;
pub(crate) mod kb_info;
pub(crate) mod dialog_info;
pub(crate) mod doc_info;
pub(crate) mod user_info;

#[derive(Debug, Deserialize, Serialize)]
struct JsonResponse<T> {
    code: u32,
    err: String,
    data: T,
}
