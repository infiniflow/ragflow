use std::collections::HashMap;
use actix_web::{ HttpResponse, post, web };
use serde::Deserialize;
use serde_json::Value;
use serde_json::json;
use crate::api::JsonResponse;
use crate::AppState;
use crate::errors::AppError;
use crate::service::dialog_info::Query;
use crate::service::dialog_info::Mutation;

#[derive(Debug, Deserialize)]
pub struct ListParams {
    pub uid: i64,
    pub dialog_id: Option<i64>,
}
#[post("/v1.0/dialogs")]
async fn list(
    params: web::Json<ListParams>,
    data: web::Data<AppState>
) -> Result<HttpResponse, AppError> {
    let mut result = HashMap::new();
    if let Some(dia_id) = params.dialog_id {
        let dia = Query::find_dialog_info_by_id(&data.conn, dia_id).await?.unwrap();
        let kb = crate::service::kb_info::Query
            ::find_kb_info_by_id(&data.conn, dia.kb_id).await?
            .unwrap();
        print!("{:?}", dia.history);
        let hist: Value = serde_json::from_str(&dia.history)?;
        let detail =
            json!({
            "dialog_id": dia_id,
            "dialog_name": dia.dialog_name.to_owned(),
            "created_at": dia.created_at.to_string().to_owned(),
            "updated_at": dia.updated_at.to_string().to_owned(),
            "history": hist,
            "kb_info": kb
        });

        result.insert("dialogs", vec![detail]);
    } else {
        let mut dias = Vec::<Value>::new();
        for dia in Query::find_dialog_infos_by_uid(&data.conn, params.uid).await? {
            let kb = crate::service::kb_info::Query
                ::find_kb_info_by_id(&data.conn, dia.kb_id).await?
                .unwrap();
            let hist: Value = serde_json::from_str(&dia.history)?;
            dias.push(
                json!({
                "dialog_id": dia.dialog_id,
                "dialog_name": dia.dialog_name.to_owned(),
                "created_at": dia.created_at.to_string().to_owned(),
                "updated_at": dia.updated_at.to_string().to_owned(),
                "history": hist,
                "kb_info": kb
            })
            );
        }
        result.insert("dialogs", dias);
    }
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

#[derive(Debug, Deserialize)]
pub struct RmParams {
    pub uid: i64,
    pub dialog_id: i64,
}
#[post("/v1.0/delete_dialog")]
async fn delete(
    params: web::Json<RmParams>,
    data: web::Data<AppState>
) -> Result<HttpResponse, AppError> {
    let _ = Mutation::delete_dialog_info(&data.conn, params.dialog_id).await?;

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
pub struct CreateParams {
    pub uid: i64,
    pub dialog_id: Option<i64>,
    pub kb_id: i64,
    pub name: String,
}
#[post("/v1.0/create_dialog")]
async fn create(
    param: web::Json<CreateParams>,
    data: web::Data<AppState>
) -> Result<HttpResponse, AppError> {
    let mut result = HashMap::new();
    if let Some(dia_id) = param.dialog_id {
        result.insert("dialog_id", dia_id);
        let dia = Query::find_dialog_info_by_id(&data.conn, dia_id).await?;
        let _ = Mutation::update_dialog_info_by_id(
            &data.conn,
            dia_id,
            &param.name,
            &dia.unwrap().history
        ).await?;
    } else {
        let dia = Mutation::create_dialog_info(
            &data.conn,
            param.uid,
            param.kb_id,
            &param.name
        ).await?;
        result.insert("dialog_id", dia.dialog_id.unwrap());
    }

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

#[derive(Debug, Deserialize)]
pub struct UpdateHistoryParams {
    pub uid: i64,
    pub dialog_id: i64,
    pub history: Value,
}
#[post("/v1.0/update_history")]
async fn update_history(
    param: web::Json<UpdateHistoryParams>,
    data: web::Data<AppState>
) -> Result<HttpResponse, AppError> {
    let mut json_response = JsonResponse {
        code: 200,
        err: "".to_owned(),
        data: (),
    };

    if let Some(dia) = Query::find_dialog_info_by_id(&data.conn, param.dialog_id).await? {
        let _ = Mutation::update_dialog_info_by_id(
            &data.conn,
            param.dialog_id,
            &dia.dialog_name,
            &param.history.to_string()
        ).await?;
    } else {
        json_response.code = 500;
        json_response.err = "Can't find dialog data!".to_owned();
    }

    Ok(
        HttpResponse::Ok()
            .content_type("application/json")
            .body(serde_json::to_string(&json_response)?)
    )
}
