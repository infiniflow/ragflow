use std::collections::HashMap;
use actix_web::{get, HttpResponse, post, web};
use crate::api::JsonResponse;
use crate::AppState;
use crate::entity::kb_info;
use crate::errors::AppError;
use crate::service::kb_info::Mutation;
use crate::service::kb_info::Query;

#[post("/v1.0/create_kb")]
async fn create(model: web::Json<kb_info::Model>, data: web::Data<AppState>) -> Result<HttpResponse, AppError> {
    let model = Mutation::create_kb_info(&data.conn, model.into_inner()).await?;

    let mut result = HashMap::new();
    result.insert("kb_id", model.kb_id.unwrap());

    let json_response = JsonResponse {
        code: 200,
        err: "".to_owned(),
        data: result,
    };

    Ok(HttpResponse::Ok()
        .content_type("application/json")
        .body(serde_json::to_string(&json_response)?))
}

#[get("/v1.0/kbs")]
async fn list(model: web::Json<kb_info::Model>, data: web::Data<AppState>) -> Result<HttpResponse, AppError> {
    let kbs = Query::find_kb_infos_by_uid(&data.conn, model.uid).await?;

    let mut result = HashMap::new();
    result.insert("kbs", kbs);

    let json_response = JsonResponse {
        code: 200,
        err: "".to_owned(),
        data: result,
    };

    Ok(HttpResponse::Ok()
        .content_type("application/json")
        .body(serde_json::to_string(&json_response)?))
}

#[post("/v1.0/delete_kb")]
async fn delete(model: web::Json<kb_info::Model>, data: web::Data<AppState>) -> Result<HttpResponse, AppError> {
    let _ = Mutation::delete_kb_info(&data.conn, model.kb_id).await?;

    let json_response = JsonResponse {
        code: 200,
        err: "".to_owned(),
        data: (),
    };

    Ok(HttpResponse::Ok()
        .content_type("application/json")
        .body(serde_json::to_string(&json_response)?))
}