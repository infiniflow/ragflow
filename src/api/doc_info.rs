use std::collections::HashMap;
use std::io::Write;
use actix_multipart_extract::{ File, Multipart, MultipartForm };
use actix_web::{ HttpResponse, post, web };
use chrono::{ Utc, FixedOffset };
use sea_orm::DbConn;
use crate::api::JsonResponse;
use crate::AppState;
use crate::entity::doc_info::Model;
use crate::errors::AppError;
use crate::service::doc_info::{ Mutation, Query };
use serde::Deserialize;

fn now() -> chrono::DateTime<FixedOffset> {
    Utc::now().with_timezone(&FixedOffset::east_opt(3600 * 8).unwrap())
}

#[derive(Debug, Deserialize)]
pub struct ListParams {
    pub uid: i64,
    pub filter: FilterParams,
    pub sortby: String,
    pub page: Option<u32>,
    pub per_page: Option<u32>,
}

#[derive(Debug, Deserialize)]
pub struct FilterParams {
    pub keywords: Option<String>,
    pub folder_id: Option<i64>,
    pub tag_id: Option<i64>,
    pub kb_id: Option<i64>,
}

#[post("/v1.0/docs")]
async fn list(
    params: web::Json<ListParams>,
    data: web::Data<AppState>
) -> Result<HttpResponse, AppError> {
    let docs = Query::find_doc_infos_by_params(&data.conn, params.into_inner()).await?;

    let mut result = HashMap::new();
    result.insert("docs", docs);

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

#[derive(Deserialize, MultipartForm, Debug)]
pub struct UploadForm {
    #[multipart(max_size = 512MB)]
    file_field: File,
    uid: i64,
    did: i64,
}

#[post("/v1.0/upload")]
async fn upload(
    payload: Multipart<UploadForm>,
    data: web::Data<AppState>
) -> Result<HttpResponse, AppError> {
    let uid = payload.uid;
    async fn add_number_to_filename(
        file_name: String,
        conn: &DbConn,
        uid: i64,
        parent_id: i64
    ) -> String {
        let mut i = 0;
        let mut new_file_name = file_name.to_string();
        let arr: Vec<&str> = file_name.split(".").collect();
        let suffix = String::from(arr[arr.len() - 1]);
        let preffix = arr[..arr.len() - 1].join(".");
        let mut docs = Query::find_doc_infos_by_name(
            conn,
            uid,
            &new_file_name,
            Some(parent_id)
        ).await.unwrap();
        while docs.len() > 0 {
            i += 1;
            new_file_name = format!("{}_{}.{}", preffix, i, suffix);
            docs = Query::find_doc_infos_by_name(
                conn,
                uid,
                &new_file_name,
                Some(parent_id)
            ).await.unwrap();
        }
        new_file_name
    }
    let fnm = add_number_to_filename(
        payload.file_field.name.clone(),
        &data.conn,
        uid,
        payload.did
    ).await;

    std::fs::create_dir_all(format!("./upload/{}/", uid));
    let filepath = format!("./upload/{}/{}-{}", payload.uid, payload.did, fnm.clone());
    let mut f = std::fs::File::create(&filepath)?;
    f.write(&payload.file_field.bytes)?;

    let doc = Mutation::create_doc_info(&data.conn, Model {
        did: Default::default(),
        uid: uid,
        doc_name: fnm,
        size: payload.file_field.bytes.len() as i64,
        location: filepath,
        r#type: "doc".to_string(),
        created_at: now(),
        updated_at: now(),
        is_deleted: Default::default(),
    }).await?;

    let _ = Mutation::place_doc(&data.conn, payload.did, doc.did.unwrap()).await?;

    Ok(HttpResponse::Ok().body("File uploaded successfully"))
}

#[derive(Deserialize, Debug)]
pub struct RmDocsParam {
    uid: i64,
    dids: Vec<i64>,
}
#[post("/v1.0/delete_docs")]
async fn delete(
    params: web::Json<RmDocsParam>,
    data: web::Data<AppState>
) -> Result<HttpResponse, AppError> {
    let _ = Mutation::delete_doc_info(&data.conn, &params.dids).await?;

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

#[derive(Debug, Deserialize)]
pub struct MvParams {
    pub uid: i64,
    pub dids: Vec<i64>,
    pub dest_did: i64,
}

#[post("/v1.0/mv_docs")]
async fn mv(
    params: web::Json<MvParams>,
    data: web::Data<AppState>
) -> Result<HttpResponse, AppError> {
    Mutation::mv_doc_info(&data.conn, params.dest_did, &params.dids).await?;

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

#[derive(Debug, Deserialize)]
pub struct NewFoldParams {
    pub uid: i64,
    pub parent_id: i64,
    pub name: String,
}

#[post("/v1.0/new_folder")]
async fn new_folder(
    params: web::Json<NewFoldParams>,
    data: web::Data<AppState>
) -> Result<HttpResponse, AppError> {
    let doc = Mutation::create_doc_info(&data.conn, Model {
        did: Default::default(),
        uid: params.uid,
        doc_name: params.name.to_string(),
        size: 0,
        r#type: "folder".to_string(),
        location: "".to_owned(),
        created_at: now(),
        updated_at: now(),
        is_deleted: Default::default(),
    }).await?;
    let _ = Mutation::place_doc(&data.conn, params.parent_id, doc.did.unwrap()).await?;

    Ok(HttpResponse::Ok().body("Folder created successfully"))
}

#[derive(Debug, Deserialize)]
pub struct RenameParams {
    pub uid: i64,
    pub did: i64,
    pub name: String,
}

#[post("/v1.0/rename")]
async fn rename(
    params: web::Json<RenameParams>,
    data: web::Data<AppState>
) -> Result<HttpResponse, AppError> {
    let docs = Query::find_doc_infos_by_name(&data.conn, params.uid, &params.name, None).await?;
    if docs.len() > 0 {
        let json_response = JsonResponse {
            code: 500,
            err: "Name duplicated!".to_owned(),
            data: (),
        };
        return Ok(
            HttpResponse::Ok()
                .content_type("application/json")
                .body(serde_json::to_string(&json_response)?)
        );
    }
    let doc = Mutation::rename(&data.conn, params.did, &params.name).await?;

    let json_response = JsonResponse {
        code: 200,
        err: "".to_owned(),
        data: doc,
    };

    Ok(
        HttpResponse::Ok()
            .content_type("application/json")
            .body(serde_json::to_string(&json_response)?)
    )
}
