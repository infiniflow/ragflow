use std::collections::HashMap;
use std::io::SeekFrom;
use std::ptr::null;
use actix_identity::Identity;
use actix_web::{HttpResponse, post, web};
use chrono::{FixedOffset, Utc};
use sea_orm::ActiveValue::NotSet;
use serde::{Deserialize, Serialize};
use crate::api::JsonResponse;
use crate::AppState;
use crate::entity::{doc_info, tag_info};
use crate::entity::user_info::Model;
use crate::errors::{AppError, UserError};
use crate::service::user_info::Mutation;
use crate::service::user_info::Query;

fn now()->chrono::DateTime<FixedOffset>{
    Utc::now().with_timezone(&FixedOffset::east_opt(3600*8).unwrap())
}

pub(crate) fn create_auth_token(user: &Model) -> u64 {
    use std::{
        collections::hash_map::DefaultHasher,
        hash::{Hash, Hasher},
    };

    let mut hasher = DefaultHasher::new();
    user.hash(&mut hasher);
    hasher.finish()
}

#[derive(Clone, Debug, Serialize, Deserialize)]
pub(crate) struct LoginParams {
    pub(crate) email: String,
    pub(crate) password: String,
}

#[post("/v1.0/login")]
async fn login(
    data: web::Data<AppState>,
    identity: Identity,
    input: web::Json<LoginParams>
) -> Result<HttpResponse, AppError> {
    match Query::login(&data.conn, &input.email, &input.password).await? {
        Some(user) => {
            let _ = Mutation::update_login_status(user.uid,&data.conn).await?;
            let token = create_auth_token(&user).to_string();

            identity.remember(token.clone());

            let json_response = JsonResponse {
                code: 200,
                err: "".to_owned(),
                data: token.clone(),
            };

            Ok(HttpResponse::Ok()
                .content_type("application/json")
                .append_header(("X-Auth-Token", token))
                .body(serde_json::to_string(&json_response)?))
        }
        None => Err(UserError::LoginFailed.into())
    }
}

#[post("/v1.0/register")]
async fn register(model: web::Json<Model>, data: web::Data<AppState>) -> Result<HttpResponse, AppError> {
    let mut result = HashMap::new();
    let u = Query::find_user_infos(&data.conn, &model.email).await?;
    if let Some(_) = u {
        let json_response = JsonResponse {
            code: 500,
            err: "Email registered!".to_owned(),
            data: (),
        };
        return Ok(HttpResponse::Ok()
            .content_type("application/json")
            .body(serde_json::to_string(&json_response)?));
    }

    let usr = Mutation::create_user(&data.conn, &model).await?;
    result.insert("uid", usr.uid.clone().unwrap());
    crate::service::doc_info::Mutation::create_doc_info(&data.conn, doc_info::Model{
        did:Default::default(),
        uid:  usr.uid.clone().unwrap(),
        doc_name: "/".into(),
        size: 0,
        location: "".into(),
        r#type: "folder".to_string(),
        created_at: now(),
        updated_at: now(),
        is_deleted:Default::default(),
    }).await?;
    let tnm = vec!["视频","图片","音乐","文档"];
    let tregx = vec![
        ".*\\.(mpg|mpeg|avi|rm|rmvb|mov|wmv|asf|dat|asx|wvx|mpe|mpa)",
        ".*\\.(png|tif|gif|pcx|tga|exif|fpx|svg|psd|cdr|pcd|dxf|ufo|eps|ai|raw|WMF|webp|avif|apng)",
        ".*\\.(WAV|FLAC|APE|ALAC|WavPack|WV|MP3|AAC|Ogg|Vorbis|Opus)",
        ".*\\.(pdf|doc|ppt|yml|xml|htm|json|csv|txt|ini|xsl|wps|rtf|hlp)"
    ];
    for i in 0..4 {
        crate::service::tag_info::Mutation::create_tag(&data.conn, tag_info::Model{
            tid: Default::default(),
            uid: usr.uid.clone().unwrap(),
            tag_name: tnm[i].to_owned(),
            regx: tregx[i].to_owned(),
            color: (i+1).to_owned() as i16,
            icon: (i+1).to_owned() as i16,
            folder_id: 0,
            created_at: Default::default(),
            updated_at: Default::default(),
        }).await?;
    }
    let json_response = JsonResponse {
        code: 200,
        err: "".to_owned(),
        data: result,
    };

    Ok(HttpResponse::Ok()
        .content_type("application/json")
        .body(serde_json::to_string(&json_response)?))
}

#[post("/v1.0/setting")]
async fn setting(model: web::Json<Model>, data: web::Data<AppState>) -> Result<HttpResponse, AppError> {
    let _ = Mutation::update_user_by_id(&data.conn, &model).await?;
    let json_response = JsonResponse {
        code: 200,
        err: "".to_owned(),
        data: (),
    };

    Ok(HttpResponse::Ok()
        .content_type("application/json")
        .body(serde_json::to_string(&json_response)?))
}