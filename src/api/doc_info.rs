use std::collections::HashMap;
use actix_multipart::Multipart;
use actix_web::{get, HttpResponse, post, web};
use actix_web::http::Error;
use chrono::Local;
use futures_util::{AsyncWriteExt, StreamExt};
use serde::Deserialize;
use std::io::Write;
use crate::api::JsonResponse;
use crate::AppState;
use crate::entity::doc_info::Model;
use crate::service::doc_info::{Mutation, Query};

#[derive(Debug, Deserialize)]
pub struct Params {
    pub uid: i64,
    pub filter: FilterParams,
    pub sortby: String,
    pub page: u64,
    pub per_page: u64,
}

#[derive(Debug, Deserialize)]
pub struct FilterParams {
    pub keywords: Option<String>,
    pub folder_id: Option<i64>,
    pub tag_id: Option<i64>,
    pub kb_id: Option<i64>,
}

#[derive(Debug, Deserialize)]
pub struct MvParams {
    pub dids: Vec<i64>,
    pub dest_did: i64,
}

#[get("/v1.0/docs")]
async fn list(params: web::Json<Params>, data: web::Data<AppState>) -> Result<HttpResponse, Error> {
    let docs = Query::find_doc_infos_by_params(&data.conn, params.into_inner())
        .await
        .unwrap();

    let mut result = HashMap::new();
    result.insert("docs", docs);

    let json_response = JsonResponse {
        code: 200,
        err: "".to_owned(),
        data: result,
    };

    Ok(HttpResponse::Ok()
        .content_type("application/json")
        .body(serde_json::to_string(&json_response).unwrap()))
}

#[post("/v1.0/upload")]
async fn upload(mut payload: Multipart, filename: web::Data<String>, did: web::Data<i64>, uid: web::Data<i64>, data: web::Data<AppState>) -> Result<HttpResponse, Error> {
    let mut size = 0;

    while let Some(item) = payload.next().await {
        let mut field = item.unwrap();

        let filepath = format!("./uploads/{}", filename.as_str());

        let mut file = web::block(|| std::fs::File::create(filepath))
            .await
            .unwrap()
            .unwrap();

        while let Some(chunk) = field.next().await {
            let data = chunk.unwrap();
            size += data.len() as u64;
            file = web::block(move || file.write_all(&data).map(|_| file))
                .await
                .unwrap()
                .unwrap();
        }
    }

    let _ = Mutation::create_doc_info(&data.conn, Model {
        did: *did.into_inner(),
        uid: *uid.into_inner(),
        doc_name: filename.to_string(),
        size,
        kb_infos: Vec::new(),
        kb_progress: 0.0,
        location: "".to_string(),
        r#type: "".to_string(),
        created_at: Local::now().date_naive(),
        updated_at: Local::now().date_naive(),
    }).await.unwrap();

    Ok(HttpResponse::Ok().body("File uploaded successfully"))
}

#[post("/v1.0/delete_docs")]
async fn delete(doc_ids: web::Json<Vec<i64>>, data: web::Data<AppState>) -> Result<HttpResponse, Error> {
    for doc_id in doc_ids.iter() {
        let _ = Mutation::delete_doc_info(&data.conn, *doc_id).await.unwrap();
    }

    let json_response = JsonResponse {
        code: 200,
        err: "".to_owned(),
        data: (),
    };

    Ok(HttpResponse::Ok()
        .content_type("application/json")
        .body(serde_json::to_string(&json_response).unwrap()))
}

#[post("/v1.0/mv_docs")]
async fn mv(params: web::Json<MvParams>, data: web::Data<AppState>) -> Result<HttpResponse, Error> {
    Mutation::mv_doc_info(&data.conn, params.dest_did, &params.dids).await.unwrap();

    let json_response = JsonResponse {
        code: 200,
        err: "".to_owned(),
        data: (),
    };

    Ok(HttpResponse::Ok()
        .content_type("application/json")
        .body(serde_json::to_string(&json_response).unwrap()))
}