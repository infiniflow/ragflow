use std::collections::HashMap;
use actix_web::{get, HttpResponse, post, web};
use actix_web::http::Error;
use crate::api::JsonResponse;
use crate::AppState;
use crate::entity::kb_info;
use crate::service::kb_info::Mutation;
use crate::service::kb_info::Query;

#[post("/v1.0/create_kb")]
async fn create(model: web::Json<kb_info::Model>, data: web::Data<AppState>) -> Result<HttpResponse, Error> {
    let model = Mutation::create_kb_info(&data.conn, model.into_inner()).await.unwrap();

    let mut result = HashMap::new();
    result.insert("kb_id", model.kb_id.unwrap());

    let json_response = JsonResponse {
        code: 200,
        err: "".to_owned(),
        data: result,
    };

    Ok(HttpResponse::Ok()
        .content_type("application/json")
        .body(serde_json::to_string(&json_response).unwrap()))
}

#[get("/v1.0/kbs")]
async fn list(model: web::Json<kb_info::Model>, data: web::Data<AppState>) -> Result<HttpResponse, Error> {
    let kbs = Query::find_kb_infos_by_uid(&data.conn, model.uid).await.unwrap();

    let mut result = HashMap::new();
    result.insert("kbs", kbs);

    let json_response = JsonResponse {
        code: 200,
        err: "".to_owned(),
        data: result,
    };

    Ok(HttpResponse::Ok()
        .content_type("application/json")
        .body(serde_json::to_string(&json_response).unwrap()))
}

#[post("/v1.0/delete_kb")]
async fn delete(model: web::Json<kb_info::Model>, data: web::Data<AppState>) -> Result<HttpResponse, Error> {
    let _ = Mutation::delete_kb_info(&data.conn, model.kb_id).await.unwrap();

    let json_response = JsonResponse {
        code: 200,
        err: "".to_owned(),
        data: (),
    };

    Ok(HttpResponse::Ok()
        .content_type("application/json")
        .body(serde_json::to_string(&json_response).unwrap()))
}