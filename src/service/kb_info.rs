use chrono::{Local, FixedOffset, Utc};
use migration::Expr;
use sea_orm::{ActiveModelTrait, ColumnTrait, DbConn, DbErr, DeleteResult, EntityTrait, PaginatorTrait, QueryFilter, QueryOrder, UpdateResult};
use sea_orm::ActiveValue::Set;
use crate::entity::kb_info;
use crate::entity::kb2_doc;
use crate::entity::kb_info::Entity;

fn now()->chrono::DateTime<FixedOffset>{
    Utc::now().with_timezone(&FixedOffset::east_opt(3600*8).unwrap())
}
pub struct Query;

impl Query {
    pub async fn find_kb_info_by_id(db: &DbConn, id: i64) -> Result<Option<kb_info::Model>, DbErr> {
        Entity::find_by_id(id).one(db).await
    }

    pub async fn find_kb_infos(db: &DbConn) -> Result<Vec<kb_info::Model>, DbErr> {
        Entity::find().all(db).await
    }

    pub async fn find_kb_infos_by_uid(db: &DbConn, uid: i64) -> Result<Vec<kb_info::Model>, DbErr> {
        Entity::find()
            .filter(kb_info::Column::Uid.eq(uid))
            .all(db)
            .await
    }
    
    pub async fn find_kb_infos_by_name(db: &DbConn, name: String) -> Result<Vec<kb_info::Model>, DbErr> {
        Entity::find()
            .filter(kb_info::Column::KbName.eq(name))
            .all(db)
            .await
    }

    pub async fn find_kb_by_docs(db: &DbConn, doc_ids: Vec<i64>) -> Result<Vec<kb_info::Model>, DbErr> {
        let mut kbids = Vec::<i64>::new();
        for k in kb2_doc::Entity::find().filter(kb2_doc::Column::Did.is_in(doc_ids)).all(db).await?{
            kbids.push(k.kb_id);
        }
        Entity::find().filter(kb_info::Column::KbId.is_in(kbids)).all(db).await
    }

    pub async fn find_kb_infos_in_page(
        db: &DbConn,
        page: u64,
        posts_per_page: u64,
    ) -> Result<(Vec<kb_info::Model>, u64), DbErr> {
        // Setup paginator
        let paginator = Entity::find()
            .order_by_asc(kb_info::Column::KbId)
            .paginate(db, posts_per_page);
        let num_pages = paginator.num_pages().await?;

        // Fetch paginated posts
        paginator.fetch_page(page - 1).await.map(|p| (p, num_pages))
    }
}

pub struct Mutation;

impl Mutation {
    pub async fn create_kb_info(
        db: &DbConn,
        form_data: kb_info::Model,
    ) -> Result<kb_info::ActiveModel, DbErr> {
        kb_info::ActiveModel {
            kb_id: Default::default(),
            uid: Set(form_data.uid.to_owned()),
            kb_name: Set(form_data.kb_name.to_owned()),
            icon: Set(form_data.icon.to_owned()),
            created_at: Set(now()),
            updated_at: Set(now()),
            is_deleted:Default::default()
        }
            .save(db)
            .await
    }

    pub async fn add_docs(
        db: &DbConn,
        kb_id: i64,
        doc_ids: Vec<i64>
    )-> Result<(), DbErr>  {
        for did in doc_ids{
            let res = kb2_doc::Entity::find()
                .filter(kb2_doc::Column::KbId.eq(kb_id))
                .filter(kb2_doc::Column::Did.eq(did))
                .all(db)
                .await?;
            if res.len()>0{continue;}
            let _ = kb2_doc::ActiveModel {
                id: Default::default(),
                kb_id: Set(kb_id),
                did: Set(did),
                kb_progress: Set(0.0),
                kb_progress_msg: Set("".to_owned()),
                updated_at: Set(now()),
                is_deleted:Default::default()
            }
                .save(db)
                .await?;
        }

        Ok(())
    }

    pub async fn remove_docs(
        db: &DbConn,
        doc_ids: Vec<i64>,
        kb_id: Option<i64>
    )-> Result<UpdateResult, DbErr>  {
        let update = kb2_doc::Entity::update_many()
            .col_expr(kb2_doc::Column::IsDeleted, Expr::value(true))
            .col_expr(kb2_doc::Column::KbProgress, Expr::value(0))
            .col_expr(kb2_doc::Column::KbProgressMsg, Expr::value(""))
            .filter(kb2_doc::Column::Did.is_in(doc_ids));
        if let Some(kbid) = kb_id{
            update.filter(kb2_doc::Column::KbId.eq(kbid))
            .exec(db)
            .await
        }else{
            update.exec(db).await
        }
    }

    pub async fn update_kb_info_by_id(
        db: &DbConn,
        id: i64,
        form_data: kb_info::Model,
    ) -> Result<kb_info::Model, DbErr> {
        let kb_info: kb_info::ActiveModel = Entity::find_by_id(id)
            .one(db)
            .await?
            .ok_or(DbErr::Custom("Cannot find.".to_owned()))
            .map(Into::into)?;

        kb_info::ActiveModel {
            kb_id: kb_info.kb_id,
            uid: kb_info.uid,
            kb_name: Set(form_data.kb_name.to_owned()),
            icon: Set(form_data.icon.to_owned()),
            created_at: kb_info.created_at,
            updated_at: Set(now()),
            is_deleted: Default::default()
        }
            .update(db)
            .await
    }

    pub async fn delete_kb_info(db: &DbConn, kb_id: i64) -> Result<DeleteResult, DbErr> {
        let kb: kb_info::ActiveModel = Entity::find_by_id(kb_id)
            .one(db)
            .await?
            .ok_or(DbErr::Custom("Cannot find.".to_owned()))
            .map(Into::into)?;

            kb.delete(db).await
    }

    pub async fn delete_all_kb_infos(db: &DbConn) -> Result<DeleteResult, DbErr> {
        Entity::delete_many().exec(db).await
    }
}
