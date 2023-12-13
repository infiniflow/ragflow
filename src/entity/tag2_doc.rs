use sea_orm::entity::prelude::*;
use serde::{Deserialize, Serialize};

#[derive(Clone, Debug, PartialEq, Eq, DeriveEntityModel, Deserialize, Serialize)]
#[sea_orm(table_name = "tag2_doc")]
pub struct Model {
    #[sea_orm(primary_key, auto_increment = false)]
    #[sea_orm(index)]
    pub tag_id: i64,
    #[sea_orm(primary_key, auto_increment = false)]
    pub uid: i64,
}

#[derive(Debug, Clone, Copy, EnumIter)]
pub enum Relation {
    DocInfo,
    Tag,
}

impl RelationTrait for Relation {
    fn def(&self) -> sea_orm::RelationDef {
        match self {
            Self::DocInfo => Entity::belongs_to(super::doc_info::Entity)
                .from(Column::Uid)
                .to(super::doc_info::Column::Uid)
                .into(),
            Self::Tag => Entity::belongs_to(super::tag_info::Entity)
                .from(Column::TagId)
                .to(super::tag_info::Column::Uid)
                .into(),
        }
    }
}

impl ActiveModelBehavior for ActiveModel {}