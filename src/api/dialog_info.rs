use std::collections::HashMap;
use actix_web::{get, HttpResponse, post, web};
use crate::api::JsonResponse;
use crate::AppState;
use crate::entity::dialog_info;
use crate::errors::AppError;
use crate::service::dialog_info::Query;
use crate::service::dialog_info::Mutation;

#[get("/v1.0/dialogs")]
async fn list(model: web::Json<dialog_info::Model>, data: web::Data<AppState>) -> Result<HttpResponse, AppError> {
    let dialogs = Query::find_dialog_infos_by_uid(&data.conn, model.uid).await?;

    let mut result = HashMap::new();
    result.insert("dialogs", dialogs);

    let json_response = JsonResponse {
        code: 200,
        err: "".to_owned(),
        data: result,
    };

    Ok(HttpResponse::Ok()
        .content_type("application/json")
        .body(serde_json::to_string(&json_response)?))
}

#[get("/v1.0/dialog")]
async fn detail(model: web::Json<dialog_info::Model>, data: web::Data<AppState>) -> Result<HttpResponse, AppError> {
    let dialogs = Query::find_dialog_info_by_id(&data.conn, model.dialog_id).await?;

    let mut result = HashMap::new();
    result.insert("dialogs", dialogs);

    let json_response = JsonResponse {
        code: 200,
        err: "".to_owned(),
        data: result,
    };

    Ok(HttpResponse::Ok()
        .content_type("application/json")
        .body(serde_json::to_string(&json_response)?))
}

#[post("/v1.0/delete_dialog")]
async fn delete(model: web::Json<dialog_info::Model>, data: web::Data<AppState>) -> Result<HttpResponse, AppError> {
    let _ = Mutation::delete_dialog_info(&data.conn, model.dialog_id).await?;

    let json_response = JsonResponse {
        code: 200,
        err: "".to_owned(),
        data: (),
    };

    Ok(HttpResponse::Ok()
        .content_type("application/json")
        .body(serde_json::to_string(&json_response)?))
}

#[post("/v1.0/create_kb")]
async fn create(model: web::Json<dialog_info::Model>, data: web::Data<AppState>) -> Result<HttpResponse, AppError> {
    let model = Mutation::create_dialog_info(&data.conn, model.into_inner()).await?;

    let mut result = HashMap::new();
    result.insert("dialog_id", model.dialog_id.unwrap());

    let json_response = JsonResponse {
        code: 200,
        err: "".to_owned(),
        data: result,
    };

    Ok(HttpResponse::Ok()
        .content_type("application/json")
        .body(serde_json::to_string(&json_response)?))
}