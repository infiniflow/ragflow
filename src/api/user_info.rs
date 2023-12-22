use std::collections::HashMap;

use actix_identity::Identity;
use actix_web::{HttpResponse, post, web};
use serde::{Deserialize, Serialize};
use crate::api::JsonResponse;
use crate::AppState;
use crate::entity::user_info::Model;
use crate::errors::{AppError, UserError};
use crate::service::user_info::Mutation;
use crate::service::user_info::Query;

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
    let usr = Mutation::create_user(&data.conn, &model).await?;
    result.insert("uid", usr.uid.unwrap());
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