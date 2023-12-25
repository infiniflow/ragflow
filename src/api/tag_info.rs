use std::collections::HashMap;
use actix_web::{ get, HttpResponse, post, web };
use serde::Deserialize;
use crate::api::JsonResponse;
use crate::AppState;
use crate::entity::tag_info;
use crate::errors::AppError;
use crate::service::tag_info::{ Mutation, Query };

#[derive(Debug, Deserialize)]
pub struct TagListParams {
    pub uid: i64,
}

#[post("/v1.0/create_tag")]
async fn create(
    model: web::Json<tag_info::Model>,
    data: web::Data<AppState>
) -> Result<HttpResponse, AppError> {
    let model = Mutation::create_tag(&data.conn, model.into_inner()).await?;

    let mut result = HashMap::new();
    result.insert("tid", model.tid.unwrap());

    let json_response = JsonResponse {
        code: 200,
        err: "".to_owned(),
        data: result,
    };

    Ok(
        HttpResponse::Ok()
            .content_type("application/json")
            .body(serde_json::to_string(&json_response)?)
    )
}

#[post("/v1.0/delete_tag")]
async fn delete(
    model: web::Json<tag_info::Model>,
    data: web::Data<AppState>
) -> Result<HttpResponse, AppError> {
    let _ = Mutation::delete_tag(&data.conn, model.tid).await?;

    let json_response = JsonResponse {
        code: 200,
        err: "".to_owned(),
        data: (),
    };

    Ok(
        HttpResponse::Ok()
            .content_type("application/json")
            .body(serde_json::to_string(&json_response)?)
    )
}

//#[get("/v1.0/tags", wrap = "HttpAuthentication::bearer(validator)")]

#[post("/v1.0/tags")]
async fn list(
    param: web::Json<TagListParams>,
    data: web::Data<AppState>
) -> Result<HttpResponse, AppError> {
    let tags = Query::find_tags_by_uid(param.uid, &data.conn).await?;

    let mut result = HashMap::new();
    result.insert("tags", tags);

    let json_response = JsonResponse {
        code: 200,
        err: "".to_owned(),
        data: result,
    };

    Ok(
        HttpResponse::Ok()
            .content_type("application/json")
            .body(serde_json::to_string(&json_response)?)
    )
}
