use std::collections::HashMap;
use actix_web::{get, HttpResponse, post, web};
use serde::Serialize;
use crate::api::JsonResponse;
use crate::AppState;
use crate::entity::kb_info;
use crate::errors::AppError;
use crate::service::kb_info::Mutation;
use crate::service::kb_info::Query;
use serde::Deserialize;

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct AddDocs2KbParams {
    pub uid: i64,
    pub dids: Vec<i64>,
    pub kb_id: i64,
}
#[post("/v1.0/create_kb")]
async fn create(model: web::Json<kb_info::Model>, data: web::Data<AppState>) -> Result<HttpResponse, AppError> {
    let mut docs = Query::find_kb_infos_by_name(&data.conn, model.kb_name.to_owned()).await.unwrap();
    if docs.len() >0 {
        let json_response = JsonResponse {
            code: 201,
            err: "Duplicated name.".to_owned(),
            data: ()
        };
        Ok(HttpResponse::Ok()
        .content_type("application/json")
        .body(serde_json::to_string(&json_response)?))
    }else{
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
}

#[post("/v1.0/add_docs_to_kb")]
async fn add_docs_to_kb(param: web::Json<AddDocs2KbParams>, data: web::Data<AppState>) -> Result<HttpResponse, AppError> {
    let _ = Mutation::add_docs(&data.conn, param.kb_id, param.dids.to_owned()).await?;

    let json_response = JsonResponse {
        code: 200,
        err: "".to_owned(),
        data: (),
    };

    Ok(HttpResponse::Ok()
        .content_type("application/json")
        .body(serde_json::to_string(&json_response)?))
}

#[post("/v1.0/anti_kb_docs")]
async fn anti_kb_docs(param: web::Json<AddDocs2KbParams>, data: web::Data<AppState>) -> Result<HttpResponse, AppError> {
    let _ = Mutation::remove_docs(&data.conn, param.dids.to_owned(), Some(param.kb_id)).await?;

    let json_response = JsonResponse {
        code: 200,
        err: "".to_owned(),
        data: (),
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

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct DocIdsParams {
    pub uid: i64,
    pub dids: Vec<i64>
}

#[post("/v1.0/all_relevents")]
async fn all_relevents(params: web::Json<DocIdsParams>, data: web::Data<AppState>) -> Result<HttpResponse, AppError> {
    let dids = crate::service::doc_info::Query::all_descendent_ids(&data.conn, &params.dids).await?;
    let mut result = HashMap::new();
    let kbs = Query::find_kb_by_docs(&data.conn, dids).await?;
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