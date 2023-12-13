use std::collections::HashMap;
use actix_web::{get, HttpResponse, post, web};
use actix_web::http::Error;
use crate::api::JsonResponse;
use crate::AppState;
use crate::entity::tag_info;
use crate::service::tag_info::{Mutation, Query};

#[post("/v1.0/create_tag")]
async fn create(model: web::Json<tag_info::Model>, data: web::Data<AppState>) -> Result<HttpResponse, Error> {
    let model = Mutation::create_tag(&data.conn, model.into_inner()).await.unwrap();

    let mut result = HashMap::new();
    result.insert("tid", model.tid.unwrap());

    let json_response = JsonResponse {
        code: 200,
        err: "".to_owned(),
        data: result,
    };

    Ok(HttpResponse::Ok()
        .content_type("application/json")
        .body(serde_json::to_string(&json_response).unwrap()))
}

#[post("/v1.0/delete_tag")]
async fn delete(model: web::Json<tag_info::Model>, data: web::Data<AppState>) -> Result<HttpResponse, Error> {
    let _ = Mutation::delete_tag(&data.conn, model.tid).await.unwrap();

    let json_response = JsonResponse {
        code: 200,
        err: "".to_owned(),
        data: (),
    };

    Ok(HttpResponse::Ok()
        .content_type("application/json")
        .body(serde_json::to_string(&json_response).unwrap()))
}

#[get("/v1.0/tags")]
async fn list(data: web::Data<AppState>) -> Result<HttpResponse, Error> {
    let tags = Query::find_tag_infos(&data.conn).await.unwrap();

    let mut result = HashMap::new();
    result.insert("tags", tags);

    let json_response = JsonResponse {
        code: 200,
        err: "".to_owned(),
        data: result,
    };

    Ok(HttpResponse::Ok()
        .content_type("application/json")
        .body(serde_json::to_string(&json_response).unwrap()))
}