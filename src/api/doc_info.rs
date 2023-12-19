use std::collections::HashMap;
use std::io::Write;
use std::slice::Chunks;
//use actix_multipart::{Multipart, MultipartError, Field};
use actix_multipart_extract::{File, Multipart, MultipartForm};
use actix_web::{get, HttpResponse, post, web};
use actix_web::web::Bytes;
use chrono::Local;
use futures_util::StreamExt;
use sea_orm::DbConn;
use crate::api::JsonResponse;
use crate::AppState;
use crate::entity::doc_info::Model;
use crate::errors::AppError;
use crate::service::doc_info::{Mutation, Query};
use serde::Deserialize;


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
async fn list(params: web::Json<Params>, data: web::Data<AppState>) -> Result<HttpResponse, AppError> {
    let docs = Query::find_doc_infos_by_params(&data.conn, params.into_inner())
        .await?;

    let mut result = HashMap::new();
    result.insert("docs", docs);

    let json_response = JsonResponse {
        code: 200,
        err: "".to_owned(),
        data: result,
    };

    Ok(HttpResponse::Ok()
        .content_type("application/json")
        .body(serde_json::to_string(&json_response)?))
}

#[derive(Deserialize, MultipartForm, Debug)]
pub struct UploadForm {
    #[multipart(max_size = 512MB)]
    file_field: File,
    uid: i64, 
    did: i64
}

#[post("/v1.0/upload")]
async fn upload(payload: Multipart<UploadForm>, data: web::Data<AppState>) -> Result<HttpResponse, AppError> {
    let uid = payload.uid;
    async fn add_number_to_filename(file_name: String, conn:&DbConn, uid:i64) -> String {
        let mut i = 0;
        let mut new_file_name = file_name.to_string();
        let arr: Vec<&str> = file_name.split(".").collect();
        let suffix = String::from(arr[arr.len()-1]);
        let preffix = arr[..arr.len()-1].join(".");
        let mut docs = Query::find_doc_infos_by_name(conn, uid, new_file_name.clone()).await.unwrap();
        while docs.len()>0 {
            i += 1;
            new_file_name = format!("{}_{}.{}", preffix, i, suffix);
            docs = Query::find_doc_infos_by_name(conn, uid, new_file_name.clone()).await.unwrap();
        }
        new_file_name
    }
    let fnm = add_number_to_filename(payload.file_field.name.clone(), &data.conn, uid).await;

    std::fs::create_dir_all(format!("./upload/{}/", uid));
    let filepath = format!("./upload/{}/{}-{}", payload.uid, payload.did, fnm.clone());
    let mut f =std::fs::File::create(&filepath)?;
    f.write(&payload.file_field.bytes)?;
    
    let doc = Mutation::create_doc_info(&data.conn, Model {
        did:Default::default(),
        uid:  uid,
        doc_name: fnm,
        size: payload.file_field.bytes.len() as i64,
        kb_infos: Vec::new(),
        kb_progress: 0.0,
        kb_progress_msg: "".to_string(),
        location: filepath,
        r#type: "doc".to_string(),
        created_at: Local::now().date_naive(),
        updated_at: Local::now().date_naive(),
    }).await?;

    let _ = Mutation::place_doc(&data.conn, payload.did, doc.did.unwrap()).await?;

    Ok(HttpResponse::Ok().body("File uploaded successfully"))
}

#[post("/v1.0/delete_docs")]
async fn delete(doc_ids: web::Json<Vec<i64>>, data: web::Data<AppState>) -> Result<HttpResponse, AppError> {
    for doc_id in doc_ids.iter() {
        let _ = Mutation::delete_doc_info(&data.conn, *doc_id).await?;
    }

    let json_response = JsonResponse {
        code: 200,
        err: "".to_owned(),
        data: (),
    };

    Ok(HttpResponse::Ok()
        .content_type("application/json")
        .body(serde_json::to_string(&json_response)?))
}

#[post("/v1.0/mv_docs")]
async fn mv(params: web::Json<MvParams>, data: web::Data<AppState>) -> Result<HttpResponse, AppError> {
    Mutation::mv_doc_info(&data.conn, params.dest_did, &params.dids).await?;

    let json_response = JsonResponse {
        code: 200,
        err: "".to_owned(),
        data: (),
    };

    Ok(HttpResponse::Ok()
        .content_type("application/json")
        .body(serde_json::to_string(&json_response)?))
}
