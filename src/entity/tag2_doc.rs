use sea_orm::entity::prelude::*;
use serde::{ Deserialize, Serialize };

#[derive(Clone, Debug, PartialEq, Eq, DeriveEntityModel, Deserialize, Serialize)]
#[sea_orm(table_name = "tag2_doc")]
pub struct Model {
    #[sea_orm(primary_key, auto_increment = true)]
    pub id: i64,
    #[sea_orm(index)]
    pub tag_id: i64,
    #[sea_orm(index)]
    pub did: i64,
}

#[derive(Debug, Clone, Copy, EnumIter)]
pub enum Relation {
    Tag,
    DocInfo,
}

impl RelationTrait for Relation {
    fn def(&self) -> sea_orm::RelationDef {
        match self {
            Self::Tag =>
                Entity::belongs_to(super::tag_info::Entity)
                    .from(Column::TagId)
                    .to(super::tag_info::Column::Tid)
                    .into(),
            Self::DocInfo =>
                Entity::belongs_to(super::doc_info::Entity)
                    .from(Column::Did)
                    .to(super::doc_info::Column::Did)
                    .into(),
        }
    }
}

impl ActiveModelBehavior for ActiveModel {}
